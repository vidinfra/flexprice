package ent

import (
	"context"
	"fmt"
	"time"

	"github.com/flexprice/flexprice/ent"
	"github.com/flexprice/flexprice/ent/wallet"
	"github.com/flexprice/flexprice/ent/wallettransaction"
	"github.com/flexprice/flexprice/internal/cache"
	walletdomain "github.com/flexprice/flexprice/internal/domain/wallet"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/postgres"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/shopspring/decimal"
)

type walletRepository struct {
	client    postgres.IClient
	logger    *logger.Logger
	queryOpts WalletTransactionQueryOptions
	cache     cache.Cache
}

func NewWalletRepository(client postgres.IClient, logger *logger.Logger, cache cache.Cache) walletdomain.Repository {
	return &walletRepository{
		client: client,
		logger: logger,
		cache:  cache,
	}
}

func (r *walletRepository) CreateWallet(ctx context.Context, w *walletdomain.Wallet) error {
	client := r.client.Querier(ctx)

	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "wallet", "create_wallet", map[string]interface{}{
		"wallet_id":   w.ID,
		"customer_id": w.CustomerID,
		"tenant_id":   w.TenantID,
	})
	defer FinishSpan(span)

	// Set environment ID from context if not already set
	if w.EnvironmentID == "" {
		w.EnvironmentID = types.GetEnvironmentID(ctx)
	}

	wallet, err := client.Wallet.Create().
		SetID(w.ID).
		SetTenantID(w.TenantID).
		SetCustomerID(w.CustomerID).
		SetName(w.Name).
		SetCurrency(w.Currency).
		SetDescription(w.Description).
		SetMetadata(w.Metadata).
		SetBalance(w.Balance).
		SetCreditBalance(w.CreditBalance).
		SetWalletStatus(string(w.WalletStatus)).
		SetAutoTopupTrigger(string(w.AutoTopupTrigger)).
		SetAutoTopupMinBalance(w.AutoTopupMinBalance).
		SetAutoTopupAmount(w.AutoTopupAmount).
		SetWalletType(string(w.WalletType)).
		SetConfig(w.Config).
		SetConversionRate(w.ConversionRate).
		SetStatus(string(w.Status)).
		SetCreatedBy(w.CreatedBy).
		SetCreatedAt(w.CreatedAt).
		SetUpdatedBy(w.UpdatedBy).
		SetUpdatedAt(w.UpdatedAt).
		SetEnvironmentID(w.EnvironmentID).
		SetAlertEnabled(w.AlertEnabled).
		SetAlertConfig(w.AlertConfig).
		SetAlertState(w.AlertState).
		Save(ctx)

	if err != nil {
		SetSpanError(span, err)
		return ierr.WithError(err).
			WithHint("Failed to create wallet").
			WithReportableDetails(map[string]interface{}{
				"customer_id": w.CustomerID,
				"currency":    w.Currency,
				"wallet_id":   w.ID,
			}).
			Mark(ierr.ErrDatabase)
	}

	// Update the input wallet with created data
	*w = *walletdomain.FromEnt(wallet)
	return nil
}

func (r *walletRepository) GetWalletByID(ctx context.Context, id string) (*walletdomain.Wallet, error) {
	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "wallet", "get_wallet_by_id", map[string]interface{}{
		"wallet_id": id,
		"tenant_id": types.GetTenantID(ctx),
	})
	defer FinishSpan(span)

	// Try to get from cache first
	if cachedWallet := r.GetCache(ctx, id); cachedWallet != nil {
		return cachedWallet, nil
	}

	client := r.client.Querier(ctx)

	w, err := client.Wallet.Query().
		Where(
			wallet.ID(id),
			wallet.TenantID(types.GetTenantID(ctx)),
			wallet.StatusEQ(string(types.StatusPublished)),
			wallet.EnvironmentID(types.GetEnvironmentID(ctx)),
		).
		Only(ctx)

	if err != nil {
		SetSpanError(span, err)
		if ent.IsNotFound(err) {
			return nil, ierr.WithError(err).
				WithHint("Wallet not found").
				WithReportableDetails(map[string]interface{}{
					"wallet_id": id,
				}).
				Mark(ierr.ErrNotFound)
		}
		return nil, ierr.WithError(err).
			WithHint("Failed to retrieve wallet").
			WithReportableDetails(map[string]interface{}{
				"wallet_id": id,
			}).
			Mark(ierr.ErrDatabase)
	}

	walletData := walletdomain.FromEnt(w)
	r.SetCache(ctx, walletData)
	return walletData, nil
}

func (r *walletRepository) GetWalletsByCustomerID(ctx context.Context, customerID string) ([]*walletdomain.Wallet, error) {
	client := r.client.Querier(ctx)

	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "wallet", "get_wallets_by_customer_id", map[string]interface{}{
		"customer_id": customerID,
		"tenant_id":   types.GetTenantID(ctx),
	})
	defer FinishSpan(span)

	wallets, err := client.Wallet.Query().
		Where(
			wallet.CustomerID(customerID),
			wallet.TenantID(types.GetTenantID(ctx)),
			wallet.StatusEQ(string(types.StatusPublished)),
			wallet.WalletStatusEQ(string(types.WalletStatusActive)),
			wallet.EnvironmentID(types.GetEnvironmentID(ctx)),
		).
		All(ctx)

	if err != nil {
		SetSpanError(span, err)
		return nil, ierr.WithError(err).
			WithHint("Failed to retrieve customer wallets").
			WithReportableDetails(map[string]interface{}{
				"customer_id": customerID,
			}).
			Mark(ierr.ErrDatabase)
	}

	result := make([]*walletdomain.Wallet, len(wallets))
	for i, w := range wallets {
		result[i] = walletdomain.FromEnt(w)
	}
	return result, nil
}

func (r *walletRepository) UpdateWalletStatus(ctx context.Context, id string, status types.WalletStatus) error {
	client := r.client.Querier(ctx)

	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "wallet", "update_wallet_status", map[string]interface{}{
		"wallet_id": id,
		"status":    status,
		"tenant_id": types.GetTenantID(ctx),
	})
	defer FinishSpan(span)

	count, err := client.Wallet.Update().
		Where(
			wallet.ID(id),
			wallet.TenantID(types.GetTenantID(ctx)),
			wallet.StatusEQ(string(types.StatusPublished)),
			wallet.EnvironmentID(types.GetEnvironmentID(ctx)),
		).
		SetWalletStatus(string(status)).
		SetUpdatedBy(types.GetUserID(ctx)).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)

	if err != nil {
		SetSpanError(span, err)
		return ierr.WithError(err).
			WithHint("Failed to update wallet status").
			WithReportableDetails(map[string]interface{}{
				"wallet_id": id,
				"status":    status,
			}).
			Mark(ierr.ErrDatabase)
	}

	if count == 0 {
		// Create an error to pass to SetSpanError instead of using ErrorBuilder directly
		notFoundErr := fmt.Errorf("wallet not found or already updated")
		SetSpanError(span, notFoundErr)

		return ierr.NewError("wallet not found or already updated").
			WithHint("The wallet may not exist or has already been updated").
			WithReportableDetails(map[string]interface{}{
				"wallet_id": id,
				"status":    status,
			}).
			Mark(ierr.ErrNotFound)
	}

	r.DeleteCache(ctx, id)
	return nil
}

// FindEligibleCredits retrieves valid credits for debit operation with pagination
// the credits are sorted by priority (lower number first), expiry date, and then by credit amount
// this is to ensure that the highest priority credits are used first, then the oldest credits,
// and if there are multiple credits with the same priority and expiry date,
// the credits with the highest credit amount are used first
func (r *walletRepository) FindEligibleCredits(ctx context.Context, walletID string, requiredAmount decimal.Decimal, pageSize int) ([]*walletdomain.Transaction, error) {
	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "wallet", "find_eligible_credits", map[string]interface{}{
		"wallet_id":       walletID,
		"required_amount": requiredAmount.String(),
		"page_size":       pageSize,
	})
	defer FinishSpan(span)

	var allCredits []*ent.WalletTransaction
	offset := 0

	for {
		credits, err := r.client.Querier(ctx).WalletTransaction.Query().
			Where(
				wallettransaction.WalletID(walletID),
				wallettransaction.EnvironmentID(types.GetEnvironmentID(ctx)),
				wallettransaction.Type(string(types.TransactionTypeCredit)),
				wallettransaction.CreditsAvailableGT(decimal.Zero),
				wallettransaction.Or(
					wallettransaction.ExpiryDateIsNil(),
					wallettransaction.ExpiryDateGTE(time.Now().UTC()),
				),
				wallettransaction.StatusEQ(string(types.StatusPublished)),
			).
			Order(
				ent.Asc(wallettransaction.FieldPriority), // Sort by priority first (nil values come last)
				ent.Asc(wallettransaction.FieldExpiryDate),
				ent.Desc(wallettransaction.FieldCreditAmount),
			).
			Offset(offset).
			Limit(pageSize).
			All(ctx)

		if err != nil {
			SetSpanError(span, err)
			return nil, ierr.WithError(err).
				WithHint("Failed to query eligible credits").
				WithReportableDetails(map[string]interface{}{
					"wallet_id":       walletID,
					"required_amount": requiredAmount,
				}).
				Mark(ierr.ErrDatabase)
		}

		if len(credits) == 0 {
			break
		}

		allCredits = append(allCredits, credits...)

		// Calculate total available so far
		var totalAvailable decimal.Decimal
		for _, c := range allCredits {
			totalAvailable = totalAvailable.Add(c.CreditsAvailable)
			if totalAvailable.GreaterThanOrEqual(requiredAmount) {
				return walletdomain.TransactionListFromEnt(allCredits), nil
			}
		}

		if len(credits) < pageSize {
			break
		}

		offset += pageSize
	}

	return walletdomain.TransactionListFromEnt(allCredits), nil
}

// ConsumeCredits processes debit operation across multiple credits
func (r *walletRepository) ConsumeCredits(ctx context.Context, credits []*walletdomain.Transaction, amount decimal.Decimal) error {
	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "wallet", "consume_credits", map[string]interface{}{
		"credits_count": len(credits),
		"amount":        amount.String(),
	})
	defer FinishSpan(span)

	remainingAmount := amount

	for _, credit := range credits {
		if remainingAmount.IsZero() {
			break
		}

		toConsume := decimal.Min(remainingAmount, credit.CreditsAvailable)
		newAvailable := credit.CreditsAvailable.Sub(toConsume)

		// Update credit's available amount
		_, err := r.client.Querier(ctx).WalletTransaction.UpdateOne(&ent.WalletTransaction{
			ID: credit.ID,
		}).
			SetCreditsAvailable(newAvailable).
			SetUpdatedAt(time.Now().UTC()).
			SetUpdatedBy(types.GetUserID(ctx)).
			Save(ctx)

		if err != nil {
			return ierr.WithError(err).
				WithHint("Failed to update credit available amount").
				WithReportableDetails(map[string]interface{}{
					"credit_id": credit.ID,
					"amount":    toConsume,
				}).
				Mark(ierr.ErrDatabase)
		}

		remainingAmount = remainingAmount.Sub(toConsume)
	}

	return nil
}

// CreateTransaction creates a new wallet transaction record
func (r *walletRepository) CreateTransaction(ctx context.Context, tx *walletdomain.Transaction) error {
	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "wallet", "create_transaction", map[string]interface{}{
		"transaction_id": tx.ID,
		"wallet_id":      tx.WalletID,
		"type":           tx.Type,
		"amount":         tx.Amount.String(),
	})
	defer FinishSpan(span)

	client := r.client.Querier(ctx)

	// Set environment ID from context if not already set
	if tx.EnvironmentID == "" {
		tx.EnvironmentID = types.GetEnvironmentID(ctx)
	}

	transaction, err := client.WalletTransaction.Create().
		SetID(tx.ID).
		SetTenantID(tx.TenantID).
		SetWalletID(tx.WalletID).
		SetType(string(tx.Type)).
		SetAmount(tx.Amount).
		SetCreditAmount(tx.CreditAmount).
		SetReferenceType(string(tx.ReferenceType)).
		SetReferenceID(tx.ReferenceID).
		SetDescription(tx.Description).
		SetMetadata(tx.Metadata).
		SetStatus(string(tx.Status)).
		SetTransactionStatus(string(tx.TxStatus)).
		SetTransactionReason(string(tx.TransactionReason)).
		SetCreditsAvailable(tx.CreditsAvailable).
		SetNillableExpiryDate(tx.ExpiryDate).
		SetCreditBalanceBefore(tx.CreditBalanceBefore).
		SetCreditBalanceAfter(tx.CreditBalanceAfter).
		SetCreatedAt(tx.CreatedAt).
		SetCreatedBy(tx.CreatedBy).
		SetUpdatedAt(tx.UpdatedAt).
		SetUpdatedBy(tx.UpdatedBy).
		SetEnvironmentID(tx.EnvironmentID).
		SetIdempotencyKey(tx.IdempotencyKey).
		SetNillablePriority(tx.Priority).
		Save(ctx)

	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to create wallet transaction").
			WithReportableDetails(map[string]interface{}{
				"wallet_id": tx.WalletID,
				"type":      tx.Type,
				"amount":    tx.Amount,
			}).
			Mark(ierr.ErrDatabase)
	}

	*tx = *walletdomain.TransactionFromEnt(transaction)
	return nil
}

// UpdateWalletBalance updates the wallet's balance and credit balance
func (r *walletRepository) UpdateWalletBalance(ctx context.Context, walletID string, finalBalance, newCreditBalance decimal.Decimal) error {
	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "wallet", "update_wallet_balance", map[string]interface{}{
		"wallet_id":          walletID,
		"final_balance":      finalBalance.String(),
		"new_credit_balance": newCreditBalance.String(),
	})
	defer FinishSpan(span)

	err := r.client.Querier(ctx).Wallet.Update().
		Where(wallet.ID(walletID)).
		SetBalance(finalBalance).
		SetCreditBalance(newCreditBalance).
		SetUpdatedBy(types.GetUserID(ctx)).
		SetUpdatedAt(time.Now().UTC()).
		Exec(ctx)

	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to update wallet balance").
			WithReportableDetails(map[string]interface{}{
				"wallet_id":      walletID,
				"balance":        finalBalance,
				"credit_balance": newCreditBalance,
			}).
			Mark(ierr.ErrDatabase)
	}
	r.DeleteCache(ctx, walletID)
	return nil
}

func (r *walletRepository) GetTransactionByID(ctx context.Context, id string) (*walletdomain.Transaction, error) {
	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "wallet", "get_transaction_by_id", map[string]interface{}{
		"transaction_id": id,
	})
	defer FinishSpan(span)

	client := r.client.Querier(ctx)
	t, err := client.WalletTransaction.Query().
		Where(
			wallettransaction.ID(id),
			wallettransaction.TenantID(types.GetTenantID(ctx)),
			wallettransaction.StatusEQ(string(types.StatusPublished)),
			wallettransaction.EnvironmentID(types.GetEnvironmentID(ctx)),
		).
		Only(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ierr.WithError(err).
				WithHint("Transaction not found").
				WithReportableDetails(map[string]interface{}{
					"transaction_id": id,
				}).
				Mark(ierr.ErrNotFound)
		}
		return nil, ierr.WithError(err).
			WithHint("Failed to retrieve transaction").
			WithReportableDetails(map[string]interface{}{
				"transaction_id": id,
			}).
			Mark(ierr.ErrDatabase)
	}

	return walletdomain.TransactionFromEnt(t), nil
}

func (r *walletRepository) ListWalletTransactions(ctx context.Context, f *types.WalletTransactionFilter) ([]*walletdomain.Transaction, error) {
	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "wallet", "list_wallet_transactions", map[string]interface{}{
		"filter": f,
	})
	defer FinishSpan(span)

	client := r.client.Querier(ctx)
	query := client.WalletTransaction.Query()

	// Apply entity-specific filters
	query = r.queryOpts.applyEntityQueryOptions(ctx, f, query)

	// Apply common query options
	query = ApplyQueryOptions(ctx, query, f, r.queryOpts)

	transactions, err := query.All(ctx)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to list wallet transactions").
			WithReportableDetails(map[string]interface{}{
				"wallet_id": f.WalletID,
			}).
			Mark(ierr.ErrDatabase)
	}

	return walletdomain.TransactionListFromEnt(transactions), nil
}

// HasTopup
func (r *walletRepository) HasTopupForPayment(ctx context.Context, paymentID string) (bool, error) {
	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "wallet", "has_topup", map[string]interface{}{
		"payment_id": paymentID,
	})
	defer FinishSpan(span)

	client := r.client.Querier(ctx)
	count, err := client.WalletTransaction.Query().
		Where(
			wallettransaction.Type(string(types.TransactionTypeCredit)),
			wallettransaction.StatusEQ(string(types.StatusPublished)),
			wallettransaction.EnvironmentID(types.GetEnvironmentID(ctx)),
			wallettransaction.ReferenceIDEQ(paymentID),
		).
		Count(ctx)

	if err != nil {
		return false, ierr.WithError(err).
			WithHint("Failed to check if the payment has topup").
			WithReportableDetails(map[string]interface{}{
				"payment_id": paymentID,
			}).
			Mark(ierr.ErrDatabase)
	}

	return count > 0, nil
}

func (r *walletRepository) ListAllWalletTransactions(ctx context.Context, f *types.WalletTransactionFilter) ([]*walletdomain.Transaction, error) {
	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "wallet", "list_all_wallet_transactions", map[string]interface{}{
		"filter": f,
	})
	defer FinishSpan(span)

	if f == nil {
		f = types.NewNoLimitWalletTransactionFilter()
	}

	client := r.client.Querier(ctx)
	query := client.WalletTransaction.Query()
	query = ApplyBaseFilters(ctx, query, f, r.queryOpts)
	if f != nil {
		query = r.queryOpts.applyEntityQueryOptions(ctx, f, query)
	}

	result, err := query.All(ctx)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to query transactions").
			WithReportableDetails(map[string]interface{}{
				"wallet_id": f.WalletID,
			}).
			Mark(ierr.ErrDatabase)
	}

	return walletdomain.TransactionListFromEnt(result), nil
}

func (r *walletRepository) CountWalletTransactions(ctx context.Context, f *types.WalletTransactionFilter) (int, error) {
	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "wallet", "count_wallet_transactions", map[string]interface{}{
		"filter": f,
	})
	defer FinishSpan(span)

	if f == nil {
		f = types.NewNoLimitWalletTransactionFilter()
	}

	client := r.client.Querier(ctx)
	query := client.WalletTransaction.Query()
	query = ApplyBaseFilters(ctx, query, f, r.queryOpts)
	if f != nil {
		query = r.queryOpts.applyEntityQueryOptions(ctx, f, query)
	}

	count, err := query.Count(ctx)
	if err != nil {
		return 0, ierr.WithError(err).
			WithHint("Failed to count wallet transactions").
			WithReportableDetails(map[string]interface{}{
				"wallet_id": f.WalletID,
			}).
			Mark(ierr.ErrDatabase)
	}

	return count, nil
}

func (r *walletRepository) UpdateTransactionStatus(ctx context.Context, id string, status types.TransactionStatus) error {
	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "wallet", "update_transaction_status", map[string]interface{}{
		"transaction_id": id,
		"status":         status,
	})
	defer FinishSpan(span)

	client := r.client.Querier(ctx)
	count, err := client.WalletTransaction.Update().
		Where(
			wallettransaction.ID(id),
			wallettransaction.TenantID(types.GetTenantID(ctx)),
			wallettransaction.StatusEQ(string(types.StatusPublished)),
			wallettransaction.EnvironmentID(types.GetEnvironmentID(ctx)),
		).
		SetTransactionStatus(string(status)).
		SetUpdatedBy(types.GetUserID(ctx)).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)

	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to update transaction status").
			WithReportableDetails(map[string]interface{}{
				"transaction_id": id,
				"status":         status,
			}).
			Mark(ierr.ErrDatabase)
	}

	if count == 0 {
		return ierr.NewError("transaction not found").
			WithHint("The transaction may not exist").
			WithReportableDetails(map[string]interface{}{
				"transaction_id": id,
			}).
			Mark(ierr.ErrNotFound)
	}

	return nil
}

// WalletTransactionQuery type alias for better readability
type WalletTransactionQuery = *ent.WalletTransactionQuery

// WalletTransactionQueryOptions implements BaseQueryOptions for wallet queries
type WalletTransactionQueryOptions struct{}

func (o WalletTransactionQueryOptions) ApplyTenantFilter(ctx context.Context, query WalletTransactionQuery) WalletTransactionQuery {
	return query.Where(wallettransaction.TenantID(types.GetTenantID(ctx)))
}

func (o WalletTransactionQueryOptions) ApplyEnvironmentFilter(ctx context.Context, query WalletTransactionQuery) WalletTransactionQuery {
	environmentID := types.GetEnvironmentID(ctx)
	if environmentID != "" {
		return query.Where(wallettransaction.EnvironmentID(environmentID))
	}
	return query
}

func (o WalletTransactionQueryOptions) ApplyStatusFilter(query WalletTransactionQuery, status string) WalletTransactionQuery {
	if status == "" {
		return query.Where(wallettransaction.StatusNotIn(string(types.StatusDeleted)))
	}
	return query.Where(wallettransaction.Status(status))
}

func (o WalletTransactionQueryOptions) ApplySortFilter(query WalletTransactionQuery, field string, order string) WalletTransactionQuery {
	orderFunc := ent.Desc
	if order == "asc" {
		orderFunc = ent.Asc
	}
	return query.Order(orderFunc(o.GetFieldName(field)))
}

func (o WalletTransactionQueryOptions) ApplyPaginationFilter(query WalletTransactionQuery, limit int, offset int) WalletTransactionQuery {
	query = query.Limit(limit)
	if offset > 0 {
		query = query.Offset(offset)
	}
	return query
}

func (o WalletTransactionQueryOptions) GetFieldName(field string) string {
	switch field {
	case "created_at":
		return wallettransaction.FieldCreatedAt
	case "updated_at":
		return wallettransaction.FieldUpdatedAt
	case "amount":
		return wallettransaction.FieldAmount
	default:
		return field
	}
}

func (o WalletTransactionQueryOptions) applyEntityQueryOptions(_ context.Context, f *types.WalletTransactionFilter, query WalletTransactionQuery) WalletTransactionQuery {
	if f == nil {
		return query
	}

	if f.WalletID != nil {
		query = query.Where(wallettransaction.WalletID(*f.WalletID))
	}

	if f.Type != nil {
		query = query.Where(wallettransaction.Type(string(*f.Type)))
	}

	if f.TransactionStatus != nil {
		query = query.Where(wallettransaction.TransactionStatus(string(*f.TransactionStatus)))
	}

	if f.ReferenceType != nil && f.ReferenceID != nil {
		query = query.Where(
			wallettransaction.ReferenceType(*f.ReferenceType),
			wallettransaction.ReferenceID(*f.ReferenceID),
		)
	}

	// Apply time range filters if specified
	if f.TimeRangeFilter != nil {
		if f.StartTime != nil {
			query = query.Where(wallettransaction.CreatedAtGTE(*f.StartTime))
		}
		if f.EndTime != nil {
			query = query.Where(wallettransaction.CreatedAtLTE(*f.EndTime))
		}
	}

	if f.CreditsAvailableGT != nil {
		query = query.Where(wallettransaction.CreditsAvailableGT(*f.CreditsAvailableGT))
	}

	if f.ExpiryDateBefore != nil {
		query = query.Where(wallettransaction.ExpiryDateLTE(*f.ExpiryDateBefore))
	}

	if f.ExpiryDateAfter != nil {
		query = query.Where(wallettransaction.ExpiryDateGTE(*f.ExpiryDateAfter))
	}

	if f.TransactionReason != nil {
		query = query.Where(wallettransaction.TransactionReason(string(*f.TransactionReason)))
	}

	if f.Priority != nil {
		query = query.Where(wallettransaction.Priority(*f.Priority))
	}

	return query
}

func (r *walletRepository) UpdateWallet(ctx context.Context, id string, w *walletdomain.Wallet) error {
	// Start a span for this repository operation
	span := StartRepositorySpan(ctx, "wallet", "update_wallet", map[string]interface{}{
		"wallet_id": id,
	})
	defer FinishSpan(span)

	client := r.client.Querier(ctx)
	update := client.Wallet.Update().
		Where(
			wallet.ID(id),
			wallet.TenantID(types.GetTenantID(ctx)),
			wallet.StatusEQ(string(types.StatusPublished)),
			wallet.EnvironmentID(types.GetEnvironmentID(ctx)),
		)

	if w.Name != "" {
		update.SetName(w.Name)
	}
	if w.Description != "" {
		update.SetDescription(w.Description)
	}
	if w.Metadata != nil {
		update.SetMetadata(w.Metadata)
	}
	if w.AutoTopupTrigger != "" {
		if w.AutoTopupTrigger == types.AutoTopupTriggerDisabled {
			// When disabling auto top-up, set all related fields to NULL
			update.SetAutoTopupTrigger(string(types.AutoTopupTriggerDisabled))
			update.ClearAutoTopupMinBalance()
			update.ClearAutoTopupAmount()
		} else {
			// When enabling auto top-up, set all required fields
			update.SetAutoTopupTrigger(string(w.AutoTopupTrigger))
			update.SetAutoTopupMinBalance(w.AutoTopupMinBalance)
			update.SetAutoTopupAmount(w.AutoTopupAmount)
		}
	}
	// Check if Config has any non-nil fields
	if w.Config.AllowedPriceTypes != nil {
		update.SetConfig(w.Config)
	}
	if w.AlertConfig != nil {
		update.SetAlertConfig(w.AlertConfig)
	}
	if w.AlertState != "" {
		update.SetAlertState(w.AlertState)
	}
	update.SetAlertEnabled(w.AlertEnabled)
	update.SetUpdatedAt(time.Now().UTC())
	update.SetUpdatedBy(types.GetUserID(ctx))

	count, err := update.Save(ctx)
	if err != nil {
		return ierr.WithError(err).
			WithHint("Failed to update wallet").
			WithReportableDetails(map[string]interface{}{
				"wallet_id": id,
			}).
			Mark(ierr.ErrDatabase)
	}

	if count == 0 {
		return ierr.NewError("wallet not found or already updated").
			WithHint("The wallet may not exist or has already been updated").
			Mark(ierr.ErrNotFound)
	}

	r.DeleteCache(ctx, id)
	return nil
}

func (r *walletRepository) SetCache(ctx context.Context, wallet *walletdomain.Wallet) {
	span := cache.StartCacheSpan(ctx, "wallet", "set", map[string]interface{}{
		"wallet_id": wallet.ID,
	})
	defer cache.FinishSpan(span)

	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)
	cacheKey := cache.GenerateKey(cache.PrefixWallet, tenantID, environmentID, wallet.ID)
	r.cache.Set(ctx, cacheKey, wallet, cache.ExpiryDefaultInMemory)
}

func (r *walletRepository) GetCache(ctx context.Context, key string) *walletdomain.Wallet {
	span := cache.StartCacheSpan(ctx, "wallet", "get", map[string]interface{}{
		"wallet_id": key,
	})
	defer cache.FinishSpan(span)

	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)
	cacheKey := cache.GenerateKey(cache.PrefixWallet, tenantID, environmentID, key)
	if value, found := r.cache.Get(ctx, cacheKey); found {
		return value.(*walletdomain.Wallet)
	}
	return nil
}

func (r *walletRepository) DeleteCache(ctx context.Context, walletID string) {
	span := cache.StartCacheSpan(ctx, "wallet", "delete", map[string]interface{}{
		"wallet_id": walletID,
	})
	defer cache.FinishSpan(span)

	tenantID := types.GetTenantID(ctx)
	environmentID := types.GetEnvironmentID(ctx)
	cacheKey := cache.GenerateKey(cache.PrefixWallet, tenantID, environmentID, walletID)
	r.cache.Delete(ctx, cacheKey)
}

func (r *walletRepository) GetWalletsByFilter(ctx context.Context, filter *types.WalletFilter) ([]*walletdomain.Wallet, error) {
	client := r.client.Querier(ctx)
	query := client.Wallet.Query().
		Where(
			wallet.StatusEQ(string(types.StatusPublished)),
			wallet.EnvironmentID(types.GetEnvironmentID(ctx)),
			wallet.TenantID(types.GetTenantID(ctx)),
		)

	// Apply status filter
	if filter.Status != nil {
		query = query.Where(wallet.WalletStatusEQ(string(*filter.Status)))
	}

	// Apply alert enabled filter
	if filter.AlertEnabled != nil {
		query = query.Where(wallet.AlertEnabledEQ(*filter.AlertEnabled))
	}

	// Apply wallet IDs filter
	if len(filter.WalletIDs) > 0 {
		query = query.Where(wallet.IDIn(filter.WalletIDs...))
	}

	wallets, err := query.All(ctx)
	if err != nil {
		return nil, ierr.WithError(err).
			WithHint("Failed to get wallets by filter").
			Mark(ierr.ErrDatabase)
	}

	return walletdomain.FromEntList(wallets), nil
}
