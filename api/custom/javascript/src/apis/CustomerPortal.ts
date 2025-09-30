/* tslint:disable */
/* eslint-disable */
/**
 * FlexPrice Customer Portal - Simple Dashboard API
 * 
 * A clean, simple interface for fetching customer dashboard data
 * with customizable options for limits, sorting, and filtering.
 */

import * as runtime from '../runtime';
import type {
    DtoCustomerUsageSummaryResponse,
    DtoCustomerEntitlementsResponse,
    DtoWalletBalanceResponse,
    DtoListSubscriptionsResponse,
    DtoListInvoicesResponse,
    DtoSubscriptionResponse,
    DtoInvoiceResponse,
} from '../models/index';
import {
    SubscriptionsGetSubscriptionStatusEnum,
    InvoicesGetInvoiceStatusEnum,
} from './index';
import { CustomersApi } from './CustomersApi';
import { SubscriptionsApi } from './SubscriptionsApi';
import { InvoicesApi } from './InvoicesApi';
import { WalletsApi } from './WalletsApi';
import { EntitlementsApi } from './EntitlementsApi';

/**
 * Configuration options for CustomerPortal
 */
export interface CustomerPortalConfig {
    /** API base path */
    basePath?: string;
    /** API key for authentication */
    apiKey?: string | Promise<string> | ((name: string) => string | Promise<string>);
    /** Access token for authentication */
    accessToken?: string | Promise<string> | ((name?: string, scopes?: string[]) => string | Promise<string>);
    /** Additional headers */
    headers?: { [key: string]: string };
    /** Custom fetch implementation */
    fetchApi?: any;
}

/**
 * Options for dashboard data fetching
 */
export interface DashboardOptions {
    /** Number of subscriptions to fetch (default: 10) */
    subscriptionLimit?: number;
    /** Number of invoices to fetch (default: 5) */
    invoiceLimit?: number;
    /** Subscription status filter (default: ['ACTIVE']) */
    subscriptionStatus?: SubscriptionsGetSubscriptionStatusEnum[];
    /** Invoice status filter (default: ['FINALIZED']) */
    invoiceStatus?: InvoicesGetInvoiceStatusEnum[];
    /** Time range - last N days */
    days?: number;
    /** Start date for filtering */
    startDate?: string;
    /** End date for filtering */
    endDate?: string;
    /** Include real-time wallet balance */
    includeRealTimeBalance?: boolean;
}

/**
 * Dashboard data response structure
 */
export interface CustomerDashboardData {
    /** Customer usage summary */
    usage?: DtoCustomerUsageSummaryResponse;
    /** Customer entitlements */
    entitlements?: DtoCustomerEntitlementsResponse;
    /** Customer wallet balance */
    walletBalance?: DtoWalletBalanceResponse;
    /** Active subscriptions */
    activeSubscriptions?: DtoSubscriptionResponse[];
    /** Recent invoices */
    invoices?: DtoInvoiceResponse[];
    /** Metadata */
    metadata: {
        fetchedAt: string;
        customerId: string;
        totalSubscriptions?: number;
        totalInvoices?: number;
        errors?: string[];
    };
}

/**
 * Individual method response types
 */
export interface UsageResponse {
    data?: DtoCustomerUsageSummaryResponse;
    error?: string;
}

export interface EntitlementsResponse {
    data?: DtoCustomerEntitlementsResponse;
    error?: string;
}

export interface WalletBalanceResponse {
    data?: DtoWalletBalanceResponse;
    error?: string;
}

export interface ActiveSubscriptionsResponse {
    data?: DtoSubscriptionResponse[];
    error?: string;
}

export interface RecentInvoicesResponse {
    data?: DtoInvoiceResponse[];
    error?: string;
}

/**
 * Customer Portal class for dashboard data
 */
export class CustomerPortal {
    private customersApi: CustomersApi;
    private subscriptionsApi: SubscriptionsApi;
    private invoicesApi: InvoicesApi;
    private walletsApi: WalletsApi;
    private entitlementsApi: EntitlementsApi;
    private customerId: string;

    constructor(customerId: string, config?: CustomerPortalConfig) {
        this.customerId = customerId;

        // Create configuration
        const configuration = new runtime.Configuration({
            basePath: config?.basePath,
            apiKey: config?.apiKey,
            accessToken: config?.accessToken,
            headers: config?.headers,
            fetchApi: config?.fetchApi,
        });

        // Initialize API instances
        this.customersApi = new CustomersApi(configuration);
        this.subscriptionsApi = new SubscriptionsApi(configuration);
        this.invoicesApi = new InvoicesApi(configuration);
        this.walletsApi = new WalletsApi(configuration);
        this.entitlementsApi = new EntitlementsApi(configuration);
    }

    /**
     * Get complete dashboard data
     */
    async getDashboardData(options: DashboardOptions = {}): Promise<CustomerDashboardData> {
        const opts = {
            subscriptionLimit: 10,
            invoiceLimit: 5,
            subscriptionStatus: [SubscriptionsGetSubscriptionStatusEnum.ACTIVE],
            invoiceStatus: [InvoicesGetInvoiceStatusEnum.FINALIZED],
            includeRealTimeBalance: true,
            ...options
        };

        const errors: string[] = [];
        const now = new Date().toISOString();

        // Calculate time range
        let startTime: string | undefined;
        let endTime: string | undefined;

        if (opts.days) {
            const start = new Date();
            start.setDate(start.getDate() - opts.days);
            startTime = start.toISOString();
            endTime = now;
        } else if (opts.startDate && opts.endDate) {
            startTime = opts.startDate;
            endTime = opts.endDate;
        }

        // Fetch all data in parallel
        const [
            usageResult,
            entitlementsResult,
            walletResult,
            subscriptionsResult,
            invoicesResult
        ] = await Promise.allSettled([
            this.getUsage(),
            this.getEntitlements(),
            this.getWalletBalance(opts.includeRealTimeBalance),
            this.getActiveSubscriptions(opts.subscriptionLimit, opts.subscriptionStatus, startTime, endTime),
            this.getRecentInvoices(opts.invoiceLimit, opts.invoiceStatus, startTime, endTime)
        ]);

        // Process results
        const usage = this.extractResult(usageResult, 'Usage');
        const entitlements = this.extractResult(entitlementsResult, 'Entitlements');
        const walletBalance = this.extractResult(walletResult, 'Wallet');
        const activeSubscriptions = this.extractResult(subscriptionsResult, 'Subscriptions');
        const invoices = this.extractResult(invoicesResult, 'Invoices');

        // Collect errors
        [usageResult, entitlementsResult, walletResult, subscriptionsResult, invoicesResult]
            .forEach((result, index) => {
                if (result.status === 'rejected') {
                    const apiNames = ['Usage', 'Entitlements', 'Wallet', 'Subscriptions', 'Invoices'];
                    errors.push(`${apiNames[index]}: ${result.reason}`);
                }
            });

        return {
            usage: usage?.data,
            entitlements: entitlements?.data,
            walletBalance: walletBalance?.data,
            activeSubscriptions: activeSubscriptions?.data,
            invoices: invoices?.data,
            metadata: {
                fetchedAt: now,
                customerId: this.customerId,
                totalSubscriptions: activeSubscriptions?.data?.length || 0,
                totalInvoices: invoices?.data?.length || 0,
                errors: errors.length > 0 ? errors : undefined,
            }
        };
    }

    /**
     * Get customer usage data
     */
    async getUsage(): Promise<UsageResponse> {
        try {
            const response = await this.customersApi.customersIdUsageGet({
                id: this.customerId
            });
            return { data: response as DtoCustomerUsageSummaryResponse };
        } catch (error) {
            return {
                error: error instanceof Error ? error.message : String(error)
            };
        }
    }

    /**
     * Get customer entitlements
     */
    async getEntitlements(): Promise<EntitlementsResponse> {
        try {
            const response = await this.customersApi.customersIdEntitlementsGet({
                id: this.customerId
            });
            return { data: response as DtoCustomerEntitlementsResponse };
        } catch (error) {
            return {
                error: error instanceof Error ? error.message : String(error)
            };
        }
    }

    /**
     * Get customer wallet balance
     */
    async getWalletBalance(includeRealTime = true): Promise<WalletBalanceResponse> {
        try {
            // First, get customer's wallets
            const walletsResponse = await this.walletsApi.customersWalletsGet({
                id: this.customerId,
                includeRealTimeBalance: includeRealTime
            });

            const wallets = (walletsResponse as any)?.data || [];
            if (wallets.length === 0) {
                return { error: 'No wallet found for customer' };
            }

            // Get the first wallet's balance
            const wallet = wallets[0];
            const balanceResponse = await this.walletsApi.walletsIdBalanceRealTimeGet({
                id: wallet.id
            });

            return { data: balanceResponse as DtoWalletBalanceResponse };
        } catch (error) {
            return {
                error: error instanceof Error ? error.message : String(error)
            };
        }
    }

    /**
     * Get active subscriptions
     */
    async getActiveSubscriptions(
        limit = 10,
        status?: SubscriptionsGetSubscriptionStatusEnum[],
        startTime?: string,
        endTime?: string
    ): Promise<ActiveSubscriptionsResponse> {
        try {
            const response = await this.subscriptionsApi.subscriptionsGet({
                customerId: this.customerId,
                limit,
                subscriptionStatus: status,
                startTime,
                endTime
            });

            const subscriptions = (response as DtoListSubscriptionsResponse)?.items || [];
            return { data: subscriptions };
        } catch (error) {
            return {
                error: error instanceof Error ? error.message : String(error)
            };
        }
    }

    /**
     * Get recent invoices
     */
    async getRecentInvoices(
        limit = 5,
        status?: InvoicesGetInvoiceStatusEnum[],
        startTime?: string,
        endTime?: string
    ): Promise<RecentInvoicesResponse> {
        try {
            const response = await this.invoicesApi.invoicesGet({
                customerId: this.customerId,
                limit,
                invoiceStatus: status,
                startTime,
                endTime
            });

            const invoices = (response as DtoListInvoicesResponse)?.items || [];
            return { data: invoices };
        } catch (error) {
            return {
                error: error instanceof Error ? error.message : String(error)
            };
        }
    }

    /**
     * Helper method to extract results from Promise.allSettled
     */
    private extractResult<T>(result: PromiseSettledResult<T>, apiName: string): T | undefined {
        if (result.status === 'fulfilled') {
            return result.value;
        } else {
            console.warn(`CustomerPortal ${apiName} API failed:`, result.reason);
            return undefined;
        }
    }
}

/**
 * Factory function to create CustomerPortal instance
 */
export function createCustomerPortal(
    customerId: string,
    config?: CustomerPortalConfig
): CustomerPortal {
    return new CustomerPortal(customerId, config);
}

/**
 * Quick one-liner function for dashboard data
 */
export async function getCustomerDashboardData(
    customerId: string,
    options?: DashboardOptions,
    config?: CustomerPortalConfig
): Promise<CustomerDashboardData> {
    const portal = new CustomerPortal(customerId, config);
    return portal.getDashboardData(options);
}
