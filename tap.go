// Package tap is a Tap Payments (tap.company, MENA) driver for togo payment.
// Set PAYMENT_DRIVER=tap + TAP_SECRET_KEY. Optional TAP_BASE_URL (defaults to the
// live API) for testing against a mock server. Auth is Bearer <secret key>.
package tap

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/togo-framework/payment"
	"github.com/togo-framework/togo"
)

const defaultAPI = "https://api.tap.company/v2"

func init() {
	payment.RegisterDriver("tap", func(k *togo.Kernel) (payment.PaymentProvider, error) {
		key := os.Getenv("TAP_SECRET_KEY")
		if key == "" {
			return nil, errors.New("payment-tap: TAP_SECRET_KEY not set")
		}
		base := os.Getenv("TAP_BASE_URL")
		if base == "" {
			base = defaultAPI
		}
		return &provider{key: key, base: strings.TrimRight(base, "/"), hc: &http.Client{Timeout: 20 * time.Second}}, nil
	})
}

type provider struct {
	key  string
	base string
	hc   *http.Client
}

func (p *provider) call(ctx context.Context, method, path string, payload any) (map[string]any, error) {
	var rdr io.Reader
	if payload != nil {
		buf, _ := json.Marshal(payload)
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.base+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var m map[string]any
	if len(b) > 0 {
		_ = json.Unmarshal(b, &m)
	}
	if resp.StatusCode >= 300 {
		return m, fmt.Errorf("tap: %s: %s", resp.Status, errMsg(m))
	}
	return m, nil
}

func status(s string) string {
	switch strings.ToUpper(s) {
	case "CAPTURED", "AUTHORIZED":
		return "succeeded"
	case "FAILED", "DECLINED", "ERROR", "CANCELLED":
		return "failed"
	default:
		return "pending"
	}
}

func (p *provider) CreateCharge(ctx context.Context, r payment.ChargeRequest) (*payment.Charge, error) {
	body := map[string]any{
		"amount":      money(r.Amount.Amount),
		"currency":    orDefault(r.Amount.Currency, "SAR"),
		"description": r.Description,
		"customer":    customer(r.Customer),
		"source":      map[string]any{"id": orDefault(r.Token, "src_all")},
		"metadata":    r.Metadata,
	}
	m, err := p.call(ctx, http.MethodPost, "/charges", body)
	if err != nil {
		return nil, err
	}
	return &payment.Charge{ID: str(m["id"]), Status: status(str(m["status"])), Amount: r.Amount, Provider: "tap", Raw: m}, nil
}

func (p *provider) Refund(ctx context.Context, r payment.RefundRequest) error {
	if r.ChargeID == "" {
		return errors.New("tap: RefundRequest.ChargeID is required")
	}
	body := map[string]any{"charge_id": r.ChargeID, "reason": "requested_by_customer"}
	if r.Amount != nil {
		body["amount"] = money(r.Amount.Amount)
		body["currency"] = orDefault(r.Amount.Currency, "SAR")
	}
	_, err := p.call(ctx, http.MethodPost, "/refunds", body)
	return err
}

func (p *provider) CreateCheckoutSession(ctx context.Context, r payment.CheckoutRequest) (*payment.CheckoutSession, error) {
	body := map[string]any{
		"amount":   money(total(r)),
		"currency": orDefault(r.Amount.Currency, "SAR"),
		"customer": customer(r.Customer),
		"source":   map[string]any{"id": "src_all"},
		"redirect": map[string]any{"url": r.SuccessURL},
		"metadata": r.Metadata,
	}
	m, err := p.call(ctx, http.MethodPost, "/charges", body)
	if err != nil {
		return nil, err
	}
	url := ""
	if tx, ok := m["transaction"].(map[string]any); ok {
		url = str(tx["url"])
	}
	return &payment.CheckoutSession{ID: str(m["id"]), URL: url}, nil
}

func (p *provider) CreateCustomer(ctx context.Context, c payment.Customer) (string, error) {
	m, err := p.call(ctx, http.MethodPost, "/customers", customer(c))
	if err != nil {
		return "", err
	}
	return str(m["id"]), nil
}

func (p *provider) CreateSubscription(context.Context, payment.SubscriptionRequest) (*payment.Subscription, error) {
	return nil, errors.New("tap: native subscriptions are not wired — use the togo subscriptions plugin")
}

// HandleWebhook parses a Tap webhook and verifies the `hashstring` header when
// TAP_SECRET_KEY can recompute it (HMAC-SHA256 over id+amount+currency+status...).
func (p *provider) HandleWebhook(_ context.Context, headers map[string]string, body []byte) (*payment.WebhookEvent, error) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("tap: bad webhook body: %w", err)
	}
	if h := header(headers, "hashstring"); h != "" {
		want := p.hash(m)
		if want != "" && !hmac.Equal([]byte(strings.ToLower(h)), []byte(strings.ToLower(want))) {
			return nil, errors.New("tap: webhook signature mismatch")
		}
	}
	return &payment.WebhookEvent{Type: "charge." + strings.ToLower(str(m["status"])), ID: str(m["id"]), Provider: "tap", Raw: m}, nil
}

// hash recomputes Tap's webhook hashstring: HMAC-SHA256(secret, "x_id...x_amount...").
func (p *provider) hash(m map[string]any) string {
	id, _ := m["id"].(string)
	status, _ := m["status"].(string)
	amount := str(m["amount"])
	currency := str(m["currency"])
	var gateway string
	if ref, ok := m["reference"].(map[string]any); ok {
		gateway = str(ref["gateway"])
	}
	toHash := fmt.Sprintf("x_id%sx_amount%sx_currency%sx_gateway_reference%sx_payment_status%s", id, amount, currency, gateway, status)
	mac := hmac.New(sha256.New, []byte(p.key))
	mac.Write([]byte(toHash))
	return hex.EncodeToString(mac.Sum(nil))
}

// ── helpers ────────────────────────────────────────────────────────────────

func money(minor int64) float64 { return float64(minor) / 100 }

func customer(c payment.Customer) map[string]any {
	out := map[string]any{}
	if c.Email != "" {
		out["email"] = c.Email
	}
	if c.Name != "" {
		out["first_name"] = c.Name
	}
	if c.Phone != "" {
		out["phone"] = map[string]any{"number": c.Phone}
	}
	return out
}

func errMsg(m map[string]any) string {
	if m == nil {
		return ""
	}
	if errs, ok := m["errors"].([]any); ok && len(errs) > 0 {
		if e, ok := errs[0].(map[string]any); ok {
			return str(e["description"])
		}
	}
	return str(m["message"])
}

func header(h map[string]string, key string) string {
	for k, v := range h {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func total(r payment.CheckoutRequest) int64 {
	if len(r.Items) == 0 {
		return r.Amount.Amount
	}
	var t int64
	for _, it := range r.Items {
		q := it.Quantity
		if q == 0 {
			q = 1
		}
		t += it.Amount.Amount * q
	}
	return t
}
