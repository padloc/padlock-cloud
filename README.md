# Padlock Cloud

Padlock Cloud is a cloud storage and synchronization service for the
[Padlock app](https://github.com/MaKleSoft/padlock/) implemented in Go. It provides a (mostly) RESTful api
for storing and retrieving a users data. Users are identified by their email addresses, which are in most
cases provided as part of the authentication credentials. Padlock Cloud does NOT implement any kind of
diffing algorithm, nor does it attempt at providing any kind of cryptographic functionality. Any encryption,
decryption and data consolidation should happen on the client side. Padlock Cloud merely provides a convenient
way of storing encrypted dumps of user data in the cloud.

## How to install/build

First, you'll need to have [Go](https://golang.org/) installed on your system. Then simply run

```sh
go get github.com/MaKleSoft/padlock-cloud
```

This will download the source code into your `$GOPATH` and automatically build and install the
`padlock-cloud` binary into `$GOPATH/bin`. Assuming you have `$GOPATH/bin` added
to your path, you should be the be able to simply run the `padlock-cloud` command from anywhere.

## Configuration

Padlock Cloud has various configuration options that can be specified through environment variables, a
config file or through command line flags. Command line takes precedence over the config file, which takes
precedence over environment variables.

```go
// Miscellaneaous options
type AppConfig struct {
	// If true, all requests via plain http will be rejected. Only https requests are allowed
	RequireTLS bool `env:"PC_REQUIRE_TLS" cli:"require-tls" yaml:"require_tls"`
	// Email address for sending error reports; Leave empty for no notifications
	NotifyEmail string `env:"PC_NOTIFY_EMAIL" cli:"notify-email" yaml:"notify_email"`
	// Path to assets directory; used for loading templates and such
	AssetsPath string `env:"PC_ASSETS_PATH" cli:"assets-path" yaml:"assets_path"`
	// Port to listen on
	Port int `env:"PC_PORT" cli:"port" yaml:"port"`
}

// LevelDB configuration
type LevelDBConfig struct {
	// Path to directory on disc where database files should be stored
	Path string `env:"PC_DB_PATH" cli:"db-path" yaml:"db_path"`
}

// Email configuration for sending activation emails and such
type EmailConfig struct {
	// User name used for authentication with the mail server
	User string `env:"PC_EMAIL_USER" cli:"email-user" yaml:"email_user"`
	// Mail server address
	Server string `env:"PC_EMAIL_SERVER" cli:"email-server" yaml:"email_server"`
	// Port on which to contact the mail server
	Port string `env:"PC_EMAIL_PORT" cli:"email-port" yaml:"email_port"`
	// Password used for authentication with the mail server
	Password string `env:"PC_EMAIL_PASSWORD" cli:"email-password" yaml:"email_password"`
}
```

### Through environment variables

```sh
export PC_REQUIRE_TLS=TRUE
export PC_NOTIFY_EMAIL=""
export PC_ASSETS_PATH=assets
export PC_PORT=3000
export PC_DB_PATH=db
export PC_EMAIL_USER=""
export PC_EMAIL_SERVER=""
export PC_EMAIL_PORT=0
export PC_EMAIL_PASSWORD=""
```

### Through a config file

A yaml file can be used by passing its path to the program via the `--config` flag:

```sh
padlock-cloud --config path/to/config.yaml
```

```yaml
---
require_tls: true
notify_email: ""
assets_path: assets
port: 3000
db_path: db
email_user: ""
email_server: ""
email_port: 0
email_password: ""

```

### Through command line

**NOTE: This is not supported yet but will be soon **

```sh
padlock-cloud \
    --require-tls true \
    --notify-email "" \
    --assets-path assets \
    --port 3000 \
    --db-path db \
    --email-user "" \
    --email-server "" \
    --email-port 0 \
    --email-password "" \
    /

```

## Troubleshooting

### Failed to load templates

```sh
2016/08/14 20:10:15 open `/some/path/templates/deprecated-version-email.txt: no such file or directory
Failed to load Template! Did you specify the correct assets path? (Currently "/some/path")
exit status 1
```

`padlock-cloud` requires various assets like templates for rendering emails, web
pages etc. These are included in this repository under the `assets` folder. When you're running
`padlock-cloud` you'll have to make sure that it knows where to find these assets. You can do that
via the `AssetsPath` option (see [Configuration](#configuration)). By default, `padlock-cloud`
will look for the templates under `$GOPATH/src/github.com/maklesoft/padlock-cloud/assets/templates`
which is where they will usually be put by the `go get` command automatically.
