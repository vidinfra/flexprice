### Configuration Implementation
As we have implemented sync with stripe....I want to have control over what entities should we sync with stripe.
As we have connections table we can add a config there which will be used to identity the entities allowed to sync.

#### Following are the entities:
1. Customer: Pull:true, PUSH: true
2. Plan:  PULL:true, PUSH : true
3. Subscription: PULL:true, PUSH: true
4. Invoices: PULL:true, PUSH :true

#### WHere will this be used ?
In webhook handler of stripe.

#### NOTES:
The implementation should be very simple to write and understand.



{
  "subscription": {
    "inbound": false,  
    "outbound": false  
  },
  "plan": {
    "inbound": false,  
    "outbound": false
  }
}