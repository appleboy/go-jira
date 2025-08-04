module github/appleboy/go-jira

go 1.23.0

require (
	github.com/andygrunwald/go-jira v1.16.0
	github.com/appleboy/com v1.1.0
	github.com/joho/godotenv v1.5.1
	github.com/russross/blackfriday/v2 v2.1.0
	github.com/yassinebenaid/godump v0.11.1
)

require (
	github.com/fatih/structs v1.1.0 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.2 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/trivago/tgo v1.0.7 // indirect
	golang.org/x/text v0.25.0 // indirect
)

replace github.com/andygrunwald/go-jira => github.com/appleboy/go-jira-lib v1.16.3
