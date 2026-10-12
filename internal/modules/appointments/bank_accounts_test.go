package appointments

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"my-app/internal/modules/bankaccounts"
)

func TestQuotationBankAccounts(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&bankaccounts.BankAccount{}))
	s := &Service{db: db}

	usd := bankaccounts.BankAccount{Label: "Mashreq USD", AccountName: "Polygraph Intl", BankName: "Mashreq", Currency: "USD", AccountNumber: "019101597642", IsDefault: true}
	aed := bankaccounts.BankAccount{Label: "Mashreq AED", AccountName: "Polygraph Intl", BankName: "Mashreq", Currency: "AED", IBAN: "AE00TEST", IsDefault: true}
	require.NoError(t, db.Create(&usd).Error)
	require.NoError(t, db.Create(&aed).Error)

	client := Client{Name: "Acme"}
	require.NoError(t, db.Create(&client).Error)

	t.Run("no selection snapshots the currency default", func(t *testing.T) {
		q := &Quotation{ClientID: client.ID, Title: "Exam", Amount: 100, Currency: "USD"}
		require.NoError(t, s.CreateQuotation(q))
		require.Len(t, q.BankAccounts, 1)
		assert.Equal(t, "019101597642", q.BankAccounts[0].AccountNumber)

		// Editing the account later must not change the issued invoice.
		require.NoError(t, db.Model(&usd).Update("account_number", "CHANGED").Error)
		got, err := s.GetQuotationByID(strconv.Itoa(int(q.ID)))
		require.NoError(t, err)
		assert.Equal(t, "019101597642", got.BankAccounts[0].AccountNumber)
	})

	t.Run("explicit picks, client-sent details ignored", func(t *testing.T) {
		q := &Quotation{
			ClientID: client.ID, Title: "Exam", Amount: 100, Currency: "USD",
			BankAccountIDs: []uint{usd.ID, aed.ID},
			BankAccounts:   []bankaccounts.Details{{AccountName: "Fraudster", IBAN: "XX"}},
		}
		require.NoError(t, s.CreateQuotation(q))
		require.Len(t, q.BankAccounts, 2)
		assert.Equal(t, "AED", q.BankAccounts[1].Currency)
	})

	t.Run("empty selection means none", func(t *testing.T) {
		q := &Quotation{ClientID: client.ID, Title: "Exam", Amount: 100, Currency: "USD", BankAccountIDs: []uint{}}
		require.NoError(t, s.CreateQuotation(q))
		assert.Empty(t, q.BankAccounts)
	})

	t.Run("legacy quote without snapshot falls back to live default", func(t *testing.T) {
		legacy := Quotation{ClientID: client.ID, Code: "INV-LEGACY", Title: "Old", Amount: 50, Currency: "AED"}
		require.NoError(t, db.Create(&legacy).Error)
		got, err := s.GetQuotationByID(strconv.Itoa(int(legacy.ID)))
		require.NoError(t, err)
		require.Len(t, got.BankAccounts, 1)
		assert.Equal(t, "AE00TEST", got.BankAccounts[0].IBAN)
	})

	t.Run("change selection later", func(t *testing.T) {
		q := &Quotation{ClientID: client.ID, Title: "Exam", Amount: 100, Currency: "USD"}
		require.NoError(t, s.CreateQuotation(q))
		got, err := s.SetQuotationBankAccounts(strconv.Itoa(int(q.ID)), []uint{aed.ID})
		require.NoError(t, err)
		require.Len(t, got.BankAccounts, 1)
		assert.Equal(t, "AED", got.BankAccounts[0].Currency)
	})
}
