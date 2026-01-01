package types

type TokenType string

const (
	TokenTypeSetupCode TokenType = "setup_code"
	TokenTypeAPIToken  TokenType = "api_token"
)

func (t TokenType) String() string {
	return string(t)
}


func (t TokenType) Values() []string {
	return []string{
		string(TokenTypeSetupCode),
		string(TokenTypeAPIToken),
	}
}