#padlock-cloud

Padlock Cloud is a cloud storage and synchronization service for the
[Padlock app](https://github.com/MaKleSoft/padlock/) implemented in Go. It provides a (mostly) RESTful api
for storing and retrieving a users data. Users are identified by their email addresses, which are in most
cases provided as part of the authentication credentials. Padlock Cloud does NOT implement any kind of
diffing algorithm, nor does it attempt at providing any kind of cryptographic functionality. Any encryption,
decryption and data consolidation should happen on the client side. Padlock Cloud merely provides a convenient
way of storing encrypted dumps of user data in the cloud.

## Installation

Padlock Cloud is currently not available as a prebuilt package. If you want to use it you have to build it from source.

1. Install [Go](https://golang.org/)
2. Clone Padlock Cloud repo

    ```sh
    git clone https://github.com/MaKleSoft/padlock-cloud.git
    cd padlock-cloud
    ```

3. Build binary

    ```sh
    go build -o padlock-cloud-bin ./padlock-cloud
    ```

4. Run it!

    ```sh
    ./padlock-cloud-bin
    ```

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

