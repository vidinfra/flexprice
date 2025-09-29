### Stripe <> Flexprice
The goal is to create a stripe subscription sync with flexprice. Flexprice will be used as usage metering platform.

#### What things we will sync with stripe
1. Meter / Feature : No
2. Plan : Yes
3. Price: No
4. Customer : YES (Already there)
5. Subscription : YES



#### Customer SYNC Workflow
1. As soon as customer is created on stripe:
    - We will create customer in flexprice
    - We will create entity mapping in flexprice customer <> stripe customer
2. As soon as customer gets deleted on stripe
    - Not yet decided
 

#### Plan SYNC Workflow
1. As soon as plan is created on stripe:
    - In flexprice we will create a empty Plan and Addon, with no price/meter etc.
    - We will create entity mapping of plan in flexprice <> plan in stripe, addon in flexprice <> plan in stripe
    - User will come to flexprice system and create meter and add it to the plan.
2. As soon as plan gets deleted:
    - We will delete plan
3. Suppose you want to update a plan price
    - Create a new PLan on stripe with updated price
    - Flexprice will sync this plan
    - Add new entitlements here
    - You will have to assign this new plan to all customers
    - Flexprice will listen this subscription update and will update customers in flexprice.

#### Subscription SYNC Workflow
1. As soon as subscription is created on stripe side:
    - In flexprice we will create a subscription here
    - We will plan from entity mapping
    - We will get customer from entity mapping
    - We will create entity mapping for subscription in flexprice <> subscription in stripe
2. As soon as the subscription is upgraded/downgraded in stripe
    - We will have also upgrade/downgrade plan to new plan
3. As soon as the subscription is updated with new line item (Addon will be not handled now)
    - We will look for that lineitem equivalent addon
    - if found we will attach it to the subscription
4. As soon as the subscription cancel/Past due/Inactive on Stripe:
    - We will cancel/Past due/Inactive

##### Cases
 1. While creating a subscription No customer found for id received from stripe
    - We can create customer while syncing (Configurable)
 2. While creating a subscription No Plan Found for the id received from stripe
    - We can create empty plan while syncing (Configurable)

