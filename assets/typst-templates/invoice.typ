#import "default.typ" as template

#let invoice-data = json(sys.inputs.path)

// Use pre-calculated totals from the service layer
#let subtotal = invoice-data.subtotal
#let total-discount = invoice-data.total_discount
#let total-tax = invoice-data.total_tax

#show: template.default-invoice.with(
  currency: if "currency" in invoice-data {
    invoice-data.currency
  },
  banner-image: if "banner_image" in invoice-data {
    image(invoice-data.banner_image, width: 30%)
  },
  invoice-status: invoice-data.invoice_status,
  invoice-number: invoice-data.invoice_number,
  issuing-date: invoice-data.issuing_date,
  due-date: invoice-data.due_date,
  amount-due: invoice-data.amount_due,
  notes: invoice-data.notes,
  subtotal: subtotal,
  discount: total-discount,
  tax: total-tax,
  biller: (
    name: invoice-data.biller.name,
    email: if "email" in invoice-data.biller {
      invoice-data.biller.email
    },
    help-email: if "help_email" in invoice-data.biller {
      invoice-data.biller.help_email
    },
    address: (
      street: invoice-data.biller.address.street,
      city: invoice-data.biller.address.city,
      postal-code: invoice-data.biller.address.postal_code,
      state: if "state" in invoice-data.biller.address {
        invoice-data.biller.address.state
      },
      country: if "country" in invoice-data.biller.address {
        invoice-data.biller.address.country
      },
    ),
    payment-instructions: if "payment_instructions" in invoice-data.biller {
      [#invoice-data.biller.payment_instructions]
    },
  ),
  recipient: (
    name: invoice-data.recipient.name,
    email: if "email" in invoice-data.recipient {
      invoice-data.recipient.email
    },
    address: (
      street: invoice-data.recipient.address.street,
      city: invoice-data.recipient.address.city,
      postal-code: invoice-data.recipient.address.postal_code,
      state: if "state" in invoice-data.recipient.address {
        invoice-data.recipient.address.state
      },
      country: if "country" in invoice-data.recipient.address {
        invoice-data.recipient.address.country
      },
    )
  ),
  items: invoice-data.line_items,
  applied-taxes: if "applied_taxes" in invoice-data {
    invoice-data.applied_taxes
  } else {
    ()
  },
  styling: (
    font: if "styling" in invoice-data and "font" in invoice-data.styling {
      invoice-data.styling.font
    } else {
      "Inter"
    },
    secondary-color: if "styling" in invoice-data and "secondary_color" in invoice-data.styling {
      invoice-data.styling.secondary_color
    } else {
      "#919191"
    },
  )
)