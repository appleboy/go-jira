package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"

	jira "github.com/andygrunwald/go-jira"
	"github.com/joho/godotenv"
)

var (
	Version     string
	Commit      string
	showVersion bool
)

func main() {
	var envfile string
	flag.StringVar(&envfile, "env-file", ".env", "Read in a file of environment variables")
	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.Parse()

	if showVersion {
		fmt.Printf("Version: %s Commit: %s\n", Version, Commit)
		return
	}

	_ = godotenv.Load(envfile)

	baseURL := getGlobalValue("base_url")
	insecure := getGlobalValue("insecure")
	username := getGlobalValue("username")
	password := getGlobalValue("password")
	token := getGlobalValue("token")

	var httpTransport *http.Transport = nil
	var httpClient *http.Client = nil

	if insecure == "true" {
		httpTransport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	if username != "" && password != "" {
		auth := jira.BasicAuthTransport{
			Username:  username,
			Password:  password,
			Transport: httpTransport,
		}
		httpClient = auth.Client()
	}

	if token != "" {
		auth := jira.BearerAuthTransport{
			Token:     token,
			Transport: httpTransport,
		}
		httpClient = auth.Client()
	}

	jiraClient, err := jira.NewClient(httpClient, baseURL)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	user, _, err := jiraClient.User.GetSelfWithContext(context.Background())
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Reporter: %s\n", user.DisplayName)
}
