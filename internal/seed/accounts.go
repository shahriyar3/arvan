package seed

import "github.com/shahriyar/arvan/internal/domain"

const (
	AccountAToken = "demo-token-account-a"
	AccountBToken = "demo-token-account-b"
)

type AccountSeed struct {
	Name  string
	Token string
}

var Accounts = []AccountSeed{
	{Name: "account-a", Token: AccountAToken},
	{Name: "account-b", Token: AccountBToken},
}

func TokenHash(token string) string {
	return domain.HashToken(token)
}
