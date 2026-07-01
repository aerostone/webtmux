package auth

import (
	"time"

	"github.com/pquerna/otp/totp"
)

func GenerateTOTPSecret(issuer string) (string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: "webtmux",
	})
	if err != nil {
		return "", err
	}
	return key.Secret(), nil
}

func VerifyTOTP(secret, code string) bool {
	return totp.Validate(code, secret)
}

func VerifyTOTPWithSkew(secret, code string, skew uint) bool {
	opts := totp.ValidateOpts{
		Period:    30,
		Skew:     skew,
		Digits:   6,
		Algorithm: 0,
	}
	valid, _ := totp.ValidateCustom(code, secret, time.Now(), opts)
	return valid
}
