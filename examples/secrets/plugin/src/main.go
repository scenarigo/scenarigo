package main

var (
	clientID     = "abcde"
	clientSecret = "XXXXX"
)

func ClientID() string {
	return clientID
}

func ClientSecretFromVault() string {
	return clientSecret
}
