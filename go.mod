module github.com/appleboy/go-jira

go 1.25.10

require (
	github.com/andygrunwald/go-jira v1.16.0
	github.com/appleboy/com v1.2.0
	github.com/joho/godotenv v1.5.1
	github.com/russross/blackfriday/v2 v2.1.0
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
	github.com/trivago/tgo v1.0.7
	github.com/yassinebenaid/godump v0.11.1
	github.com/zalando/go-keyring v0.2.8
	golang.org/x/oauth2 v0.36.0
)

require (
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.2 // indirect
	github.com/google/go-querystring v1.2.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	golang.org/x/sys v0.44.0 // indirect
	golang.org/x/text v0.37.0 // indirect
)

replace github.com/andygrunwald/go-jira => github.com/appleboy/go-jira-lib v1.16.4
