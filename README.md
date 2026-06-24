# payment-tap

[Tap](https://developers.tap.company) driver for togo **payment**.

```bash
togo install togo-framework/payment
togo install togo-framework/payment-tap
```
```env
PAYMENT_DRIVER=tap
TAP_SECRET_KEY=...
```

Registers on the togo `payment.PaymentProvider` interface and is selected via
`PAYMENT_DRIVER=tap`. Gateway API calls are scaffolded — see the Tap docs.

MIT
