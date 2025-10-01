
This document describes the synchronization flows between Stripe and Flexprice, covering Customers, Plans, and Subscriptions. The goal is to keep Flexprice in sync with Stripe for those entities, so Flexprice can act as the usage metering / billing extension.

## Overview & Objectives

- We will keep **Stripe** as a source of truth. Any update on **Flexprice** side will be overridden by the stripe entity data.
- Metered/Usage charges will be handled by **Flexprice** while stripe will handle all fixed charges.

---

## Entity Mapping Table

We maintain a 1:1 mapping table in Flexprice:

| Stripe Entity            | Flexprice Entity            | Mapping Table / Record                       |
| ------------------------ | --------------------------- | -------------------------------------------- |
| Customer (id)            | Flexprice Customer (id)     | e.g. `stripe_customer_id → flex_customer_id` |
| Stripe Plan (id)         | Flexprice Plan / Addon (id) | `stripe_plan_id → flex_plan_id`              |
| Stripe Subscription (id) | Flexprice Subscription (id) | `stripe_sub_id → flex_sub_id`                |

Every time we sync or act on an entity, we first look up the mapping.

---

## 1. Plan Sync Flow

### Trigger: Stripe → Flexprice

When a **Plan is created** in Stripe:

**Flow Steps**:

1. Receive Stripe webhook `product.created`.  
2. Check mapping: does this `stripe_plan_id` map to a Flexprice plan?  
   - If yes, skip or ensure existing link.  
3. If no mapping:  
   - Create a new **empty plan** (no meters, no pricing logic yet).  
   - Also create an “addon” entity in Flexprice mapping to the same Stripe plan (if your internal model distinguishes base plan + addon). **Implementation to be taken care in future**
   - Insert mapping: `stripe_plan_id ↔ flex_plan_id`.  
   - If you made an addon, map that as well.  
4. (Future) After user configures meters/entitlements in Flexprice, bind them to the mapped plan.

**Diagram**
![[Screenshot 2025-09-29 at 6.11.02 PM.png]]

```

```

### Updating / Deleting Plans / Price Changes

- **Plan deleted** in Stripe:  
  - On `plan.deleted` webhook (or price deletion), lookup mapping, then Flexprice will delete (or deactivate) that plan.  
  - Archive Mapping.  

- **Price change** (i.e. new plan version):  
  - Best practice: Create a *new* plan in Stripe with updated pricing.  
  - Stripe triggers plan creation → Flexprice syncs this new plan.  
  - Then you’d assign this new plan to existing customers (via subscription update) in Stripe.  
  - Flexprice sees subscription updates and updates internal subscriptions accordingly.

- If a subscription arrives for a plan id not yet mapped:  
  - Optionally auto-create an empty plan mapping in Flexprice (if configured) so subscription sync doesn’t break.![[Screenshot 2025-09-29 at 6.26.32 PM.png]]

**Note**: Because you decouple pricing logic from Flexprice, you always mirror plan but not the price itself.

---

## 2. Subscription Sync Flow

### Trigger: Stripe → Flexprice

When a **Subscription** event occurs (create, update, cancel, etc.):

**Flow Steps (on subscription.create)**:

1. Receive webhook `customer.subscription.created`.  
2. Extract `stripe_subscription_id`, `stripe_customer_id`, `stripe_plan_id(s)`, status, period info, metadata, etc.  
3. Lookup mapping:  
   - Find `flex_customer_id` for the `stripe_customer_id`  
     - If missing: if configured, auto-create the Flexprice customer; otherwise, fail or queue.  
   - Find `flex_plan_id` for the `stripe_plan_id`  
     - If missing: if configured, auto-create empty plan mapping; otherwise, fail or queue.  
4. Flexprice will create a subscription:  
   - Pass the `flex_customer_id`, `flex_plan_id`, billing period, start/end dates, metadata, etc.  
5. Receive `flex_subscription_id` and store mapping: `stripe_subscription_id ↔ flex_subscription_id`.

**Flow Steps (on subscription.update: upgrade / downgrade / plan change)**:

1. Receive webhook `customer.subscription.updated`.  
2. Lookup mapping for subscription, customer, plan.  
3. Determine new plan(s) or line items.  
4. Call Flexprice API to update the subscription in Flexprice:  
   - Change the linked plan (if plan changed).  
   - Add or remove addons (if additional line items).  
   - Update billing cycle, proration, metadata, etc.  
5. Keep mapping and store state.

**Flow Steps (on subscription.cancel / delete / status change)**:

1. Receive webhook `customer.subscription.deleted` or `subscription.updated` with status `canceled`, `unpaid`, `past_due`, etc.  
2. Lookup mapping.  
3. Call Flexprice API: cancel or mark subscription as inactive / past_due accordingly.  
4. Optionally, retain history in Flexprice for usage reporting.

**Diagram (subscription creation)**:

```

Stripe ── subscription.created webhook ──> Sync Handler  
│  
├─ lookup stripe_customer_id → flex_customer_id (or auto-create)  
│  
├─ lookup stripe_plan_id → flex_plan_id (or auto-create)  
│  
└─ call Flexprice API: create subscription(flex_customer_id, flex_plan_id, start, end, metadata)  
│  
receive flex_subscription_id  
│  
save mapping: stripe_subscription_id ↔ flex_subscription_id

```

**Diagram (subscription update / cancel)**:

```

```

### Edge / Error Cases & Behavior Options

- **Missing Customer or Plan mapping**:  
  - Option: auto-create (if enabled).  
  - Option: enqueue / delay processing until mapping exists.  

- **Partial failures**:  
  - If Flexprice API fails during subscription update, retry, log, and have reconciliation to compare Stripe vs Flexprice state.  

- **Prorations / partial periods**:  
  - If Stripe handles proration / proration invoice, pass relevant metadata / amounts in the Flexprice call so usage / billing remains consistent.  

- **Multiple line items / addons**:  
  - If a subscription includes multiple price lines, treat price lines beyond the base as “addons”.  
  - You should map Stripe addon price IDs to Flexprice addon entities; attach/detach as line items change.  

- **Subscription resume / reactivation**:  
  - Mirror transitions (e.g. `subscription.updated` status from `canceled → active`) in Flexprice.

---

## Summary & Considerations

- Sync system is mostly **reactive to Stripe webhooks**, with idempotency, retries, and reconciliation jobs built in.  
- Mapping tables are central: always map Stripe IDs to Flexprice IDs before you call Flexprice API.  
- You should allow configurable behaviors (e.g. auto-create missing customer or plan, what to do on deletions).  
- Be careful about consistency (retries, race conditions) — e.g. two webhook events for the same subscription arriving out of order.  
- A periodic **reconciliation / backfill job** is recommended: scan Stripe for all customers, plans, subscriptions, compare to Flexprice, and repair missing or inconsistent mappings.  
- Optionally, you could support Flexprice → Stripe sync (e.g. letting an admin create a plan or subscription in Flexprice which then pushes to Stripe), but that complicates the design especially around conflict resolution.






### Current Issues

1. In flexprice we cannot make subscription with empty plan
2. 






#### Stripe <> Flexprice
1. I want to sync subscription
2. I dont want to sync any line items
3. I want to capture all the plan change
4. I will only have usage charges, in my system and fixed charges 