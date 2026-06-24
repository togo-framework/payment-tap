package tap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/togo-framework/payment"
)

func newTestProvider(h http.HandlerFunc) (*provider, *httptest.Server) {
	srv := httptest.NewServer(h)
	return &provider{key: "sk_test", base: srv.URL, hc: srv.Client()}, srv
}

func TestStatus(t *testing.T) {
	for in, want := range map[string]string{"CAPTURED": "succeeded", "AUTHORIZED": "succeeded", "FAILED": "failed", "DECLINED": "failed", "INITIATED": "pending"} {
		if got := status(in); got != want {
			t.Errorf("status(%q)=%q want %q", in, got, want)
		}
	}
}

func TestCreateCharge(t *testing.T) {
	p, srv := newTestProvider(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/charges" || r.Header.Get("Authorization") != "Bearer sk_test" {
			t.Errorf("bad request %s %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["amount"].(float64) != 15.0 || body["currency"] != "SAR" {
			t.Errorf("bad body: %v", body)
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "chg_1", "status": "CAPTURED"})
	})
	defer srv.Close()
	ch, err := p.CreateCharge(context.Background(), payment.ChargeRequest{Amount: payment.Money{Amount: 1500, Currency: "SAR"}, Token: "tok_visa"})
	if err != nil {
		t.Fatal(err)
	}
	if ch.ID != "chg_1" || ch.Status != "succeeded" {
		t.Errorf("got %+v", ch)
	}
}

func TestCheckoutReturnsTransactionURL(t *testing.T) {
	p, srv := newTestProvider(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"id": "chg_2", "status": "INITIATED", "transaction": map[string]any{"url": "https://pay.tap/x"}})
	})
	defer srv.Close()
	cs, err := p.CreateCheckoutSession(context.Background(), payment.CheckoutRequest{Amount: payment.Money{Amount: 2000, Currency: "SAR"}, SuccessURL: "https://app/ok"})
	if err != nil {
		t.Fatal(err)
	}
	if cs.URL != "https://pay.tap/x" {
		t.Errorf("url = %q", cs.URL)
	}
}

func TestRefundAndCustomer(t *testing.T) {
	p, srv := newTestProvider(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/refunds":
			json.NewEncoder(w).Encode(map[string]any{"id": "ref_1", "status": "REFUNDED"})
		case "/customers":
			json.NewEncoder(w).Encode(map[string]any{"id": "cus_1"})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	defer srv.Close()
	if err := p.Refund(context.Background(), payment.RefundRequest{ChargeID: "chg_1"}); err != nil {
		t.Fatal(err)
	}
	id, err := p.CreateCustomer(context.Background(), payment.Customer{Email: "a@b.com", Name: "A"})
	if err != nil || id != "cus_1" {
		t.Errorf("customer id=%q err=%v", id, err)
	}
}

func TestWebhookHashVerification(t *testing.T) {
	p := &provider{key: "sk_test"}
	m := map[string]any{"id": "chg_9", "amount": "15.00", "currency": "SAR", "status": "CAPTURED", "reference": map[string]any{"gateway": "g1"}}
	good := p.hash(m)
	body, _ := json.Marshal(m)
	if _, err := p.HandleWebhook(context.Background(), map[string]string{"hashstring": good}, body); err != nil {
		t.Errorf("valid hash rejected: %v", err)
	}
	if _, err := p.HandleWebhook(context.Background(), map[string]string{"hashstring": "deadbeef"}, body); err == nil {
		t.Error("bad hash accepted")
	}
	// no hash header → accepted
	if ev, err := p.HandleWebhook(context.Background(), nil, body); err != nil || ev.ID != "chg_9" {
		t.Errorf("no-header path: ev=%+v err=%v", ev, err)
	}
}
