// Package tap is a Tap driver for togo payment. Blank-import it and set
// PAYMENT_DRIVER=tap plus TAP_SECRET_KEY. The driver registers and is env-configured; the
// gateway API calls are scaffolded (see Tap docs: https://developers.tap.company) — the togo payment
// interface is satisfied. Contributions to flesh out the calls are welcome.
package tap

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/togo-framework/payment"
	"github.com/togo-framework/togo"
)

func init() {
	payment.RegisterDriver("tap", func(k *togo.Kernel) (payment.PaymentProvider, error) {
		key := os.Getenv("TAP_SECRET_KEY")
		if key == "" {
			return nil, errors.New("payment-tap: TAP_SECRET_KEY not set")
		}
		return &provider{key: key, hc: &http.Client{Timeout: 20 * time.Second}}, nil
	})
}

type provider struct {
	key string
	hc  *http.Client
}

var errTODO = errors.New("payment-tap: this operation is scaffolded — wire the Tap API (https://developers.tap.company)")

func (p *provider) CreateCharge(context.Context, payment.ChargeRequest) (*payment.Charge, error) {
	return nil, errTODO
}
func (p *provider) Refund(context.Context, payment.RefundRequest) error { return errTODO }
func (p *provider) CreateCheckoutSession(context.Context, payment.CheckoutRequest) (*payment.CheckoutSession, error) {
	return nil, errTODO
}
func (p *provider) CreateCustomer(context.Context, payment.Customer) (string, error) { return "", errTODO }
func (p *provider) CreateSubscription(context.Context, payment.SubscriptionRequest) (*payment.Subscription, error) {
	return nil, errTODO
}
func (p *provider) HandleWebhook(context.Context, map[string]string, []byte) (*payment.WebhookEvent, error) {
	return nil, errTODO
}
