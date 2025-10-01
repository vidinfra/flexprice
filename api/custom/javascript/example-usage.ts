/**
 * CustomerPortal Usage Examples
 * 
 * This file demonstrates how to use the CustomerPortal class
 * for building customer dashboards.
 */

import {
    CustomerPortal,
    Configuration,
    DashboardOptions,
    SubscriptionsGetSubscriptionStatusEnum,
    InvoicesGetInvoiceStatusEnum
} from '@flexprice/javascript-sdk';

// Example 1: Basic Setup
async function basicExample() {
    // Initialize with your API configuration
    const configuration = new Configuration({
        apiKey: 'your-api-key',
        basePath: 'https://api.cloud.flexprice.io'
    });

    // Create CustomerPortal instance
    const customerPortal = new CustomerPortal(configuration);

    // Get complete dashboard data
    const dashboardData = await customerPortal.getDashboardData('customer-123', {
        subscriptionLimit: 10,
        invoiceLimit: 5,
        days: 30
    });

    console.log('Customer:', dashboardData.customer?.name);
    console.log('Active Subscriptions:', dashboardData.activeSubscriptions?.length);
    console.log('Wallet Balance:', dashboardData.walletBalance?.realTimeBalance);
}

// Example 2: Advanced Filtering
async function advancedExample() {
    const configuration = new Configuration({
        apiKey: 'your-api-key'
    });

    const customerPortal = new CustomerPortal(configuration);

    // Get dashboard with custom filters
    const data = await customerPortal.getDashboardData('customer-123', {
        subscriptionLimit: 20,
        invoiceLimit: 10,
        subscriptionStatus: [SubscriptionsGetSubscriptionStatusEnum.ACTIVE],
        invoiceStatus: [InvoicesGetInvoiceStatusEnum.FINALIZED],
        days: 90,
        includeFeatures: true,
        featureIds: ['feature-1', 'feature-2']
    });

    console.log('Filtered data:', data);
}

// Example 3: Individual Method Usage
async function individualMethodsExample() {
    const configuration = new Configuration({
        apiKey: 'your-api-key'
    });

    const customerPortal = new CustomerPortal(configuration);

    // Get specific data types
    const usage = await customerPortal.getUsage('customer-123');
    const entitlements = await customerPortal.getEntitlements('customer-123');
    const wallet = await customerPortal.getWalletBalance('customer-123');
    const subscriptions = await customerPortal.getActiveSubscriptions('customer-123');
    const invoices = await customerPortal.getRecentInvoices('customer-123');

    console.log('Usage:', usage.data);
    console.log('Entitlements:', entitlements.data);
    console.log('Wallet Balance:', wallet.data?.realTimeBalance);
    console.log('Subscriptions:', subscriptions.data?.length);
    console.log('Invoices:', invoices.data?.length);
}

// Example 4: Error Handling
async function errorHandlingExample() {
    const configuration = new Configuration({
        apiKey: 'your-api-key'
    });

    const customerPortal = new CustomerPortal(configuration);

    try {
        const dashboardData = await customerPortal.getDashboardData('customer-123');

        // Check for global errors
        if (dashboardData.metadata.errors) {
            console.error('Global errors:', dashboardData.metadata.errors);
        }

        // Check individual method responses
        const usage = await customerPortal.getUsage('customer-123');
        if (usage.error) {
            console.error('Usage fetch failed:', usage.error);
        } else {
            console.log('Usage data:', usage.data);
        }
    } catch (error) {
        console.error('Failed to fetch dashboard data:', error);
    }
}

// Example 5: React Component Usage
export function CustomerDashboardComponent({ customerId }: { customerId: string }) {
    // This would be used in a React component
    const fetchDashboardData = async () => {
        const configuration = new Configuration({
            apiKey: process.env.REACT_APP_FLEXPRICE_API_KEY
        });

        const customerPortal = new CustomerPortal(configuration);

        try {
            const dashboardData = await customerPortal.getDashboardData(customerId, {
                subscriptionLimit: 10,
                invoiceLimit: 5,
                includeFeatures: true
            });

            return dashboardData;
        } catch (error) {
            console.error('Failed to fetch dashboard data:', error);
            return null;
        }
    };

    return { fetchDashboardData };
}

// Example 6: Using Factory Functions
async function factoryFunctionsExample() {
    const configuration = new Configuration({
        apiKey: 'your-api-key'
    });

    // Using factory functions for one-liner calls
    const dashboardData = await getCustomerDashboardData('customer-123', {
        subscriptionLimit: 5,
        invoiceLimit: 3
    }, configuration);

    const summary = await getCustomerSummary('customer-123', configuration);

    console.log('Dashboard:', dashboardData);
    console.log('Summary:', summary);
}

// Example 7: Custom Configuration Options
async function customConfigurationExample() {
    const configuration = new Configuration({
        apiKey: 'your-api-key',
        basePath: 'https://api.cloud.flexprice.io',
        middleware: [
            {
                pre: (context) => {
                    console.log('API Request:', context.url);
                    return context;
                }
            }
        ]
    });

    const customerPortal = new CustomerPortal(configuration);

    const data = await customerPortal.getDashboardData('customer-123', {
        // Only fetch what you need
        includeCustomer: true,
        includeSubscriptions: true,
        includeInvoices: false,
        includeUsage: false,
        includeEntitlements: false,
        includeWalletBalance: true,
        includeSummary: false,
        includeAnalytics: false,
        includeFeatures: false,

        // Custom limits
        subscriptionLimit: 5,
        invoiceLimit: 3,

        // Time filtering
        days: 7,

        // Status filtering
        subscriptionStatus: [SubscriptionsGetSubscriptionStatusEnum.ACTIVE],
        invoiceStatus: [InvoicesGetInvoiceStatusEnum.FINALIZED]
    });

    console.log('Custom filtered data:', data);
}

// Export the examples for use
export {
    basicExample,
    advancedExample,
    individualMethodsExample,
    errorHandlingExample,
    customConfigurationExample
};
