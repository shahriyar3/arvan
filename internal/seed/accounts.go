package seed

import "github.com/shahriyar/arvan/internal/domain"

const (
	AccountAToken        = "demo-token-account-a"
	AccountBToken        = "demo-token-account-b"
	DemoInitialBalance   int64 = 10_000
)

type AccountSeed struct {
	Name           string
	Token          string
	InitialBalance int64
}

var Accounts = []AccountSeed{
	{Name: "account-a", Token: AccountAToken, InitialBalance: DemoInitialBalance},
	{Name: "account-b", Token: AccountBToken, InitialBalance: DemoInitialBalance},
}

func TokenHash(token string) string {
	return domain.HashToken(token)
}
