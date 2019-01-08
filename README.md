# Padlock Cloud

## What is Padlock Cloud

Padlock Cloud is a cloud storage service for the
[Padlock app](https://github.com/padlock/padlock/) implemented in Go. It
provides a (mostly) RESTful api for storing and retrieving user data. Padlock
Cloud does NOT implement any kind of diffing algorithm, nor does it attempt to
provide any kind of cryptographic functionality. Any encryption, decryption and
data consolidation should happen on the client side. Padlock Cloud merely
provides a cloud-based storage for encrypted user data.

## Usage

The `padlock-cloud` command provides commands for starting Padlock Cloud server
and managing accounts. It can be configured through various flags and
environment variables.

Note that **global flags** have to be specified **before** the command and
**command-specific** flags **after** the command but before any positional
arguments.

```sh
padlock-cloud [global options] command [command options] [arguments...]
```

For a list of available commands and global options, run.

```sh
padlock-cloud --help
```

For information about a specific command, including command-specific options,
run

```sh
padlock-cloud command --help
```

## Commands

### runserver
Starts a Padlock Cloud server instance

#### Environment Variables, Flags, Configuration File Variables
| Environment Variable | Flag                   | Configuration File   | Description                                  |
|----------------------|------------------------|----------------------|----------------------------------------------|
| `PC_PORT`            | `--port` &#124; `-p`   | `server.port`        | Port to listen on                            |
| `PC_ASSETS_PATH`     | `--assets-path`        | `server.assets_path` | Path to assets directory                     |
| `PC_TLS_CERT`        | `--tls-cert`           | `server.tls_cert`    | Path to TLS certification file               |
| `PC_TLS_KEY`         | `--tls-key`            | `server.tls_key`     | Path to TLS key file                         |
| `PC_BASE_URL`        | `--base-url`           | `server.base_url`    | Base url for constructing urls               |
| `PC_CORS`            | `--cors`               | `server.cors`        | Enable Cross-Origin Resource Sharing         |
| `PC_TEST`            | `--test`               |                      | Enable test mode                             |

### accounts
Commands for managing accounts.

#### list
List existing accounts.

#### create
Create new account.

#### display
Display account.

#### delete
Delete account.

### gensecret
Generate random 32 byte secret.

## Configuration
This image provides multiple options to configure the application. 

### Precedence
The precedence for flag value sources is as follows (highest to lowest):

1. Command line flag value from user
2. Environment variable (if specified)
3. Configuration file (if specified)
4. Default defined on the flag

### Environment Variables, Flags, Configuration File Variables

| Environment Variable | Flag                   | Configuration File   | Description                                  |
|----------------------|------------------------|----------------------|----------------------------------------------|
| Global                                                                                                              |
| `PC_CONFIG_PATH`     | `--config` &#124; `-c` |                      | Path to configuration file.                  |
| `PC_LOG_FILE`        | `--log-file`           | `log.log_file`       | Path to log file                             |
| `PC_ERR_FILE`        | `--err-file`           | `log.err_file`       | Path to error log file                       |
| `PC_NOTIFY_ERRORS`   | `--notify-errors`      | `log.notify_errors`  | Email address to send unexpected errors to   |
| `PC_LEVELDB_PATH`    | `--db-path`            | `leveldb.path`       | Path to LevelDB database                     |
| `PC_EMAIL_SERVER`    | `--email-server`       | `email.server`       | Mail server for sending emails               |
| `PC_EMAIL_PORT`      | `--email-port`         | `email.port`         | Port to use with mail server                 |
| `PC_EMAIL_USER`      | `--email-user`         | `email.user`         | Username for authentication with mail server |
| `PC_EMAIL_PASSWORD`  | `--email-password`     | `email.password`     | Password for authentication with mail server |
| Command: runserver                                                                                                  |
| `PC_PORT`            | `--port` &#124; `-p`   | `server.port`        | Port to listen on                            |
| `PC_ASSETS_PATH`     | `--assets-path`        | `server.assets_path` | Path to assets directory                     |
| `PC_TLS_CERT`        | `--tls-cert`           | `server.tls_cert`    | Path to TLS certification file               |
| `PC_TLS_KEY`         | `--tls-key`            | `server.tls_key`     | Path to TLS key file                         |
| `PC_BASE_URL`        | `--base-url`           | `server.base_url`    | Base url for constructing urls               |
| `PC_CORS`            | `--cors`               | `server.cors`        | Enable Cross-Origin Resource Sharing         |
| `PC_TEST`            | `--test`               |                      | Enable test mode                             |

### Configuration File

The provided file should be in the
[YAML format](http://yaml.org/). Here is an example configuration file:

```yaml
server:
  assets_path: assets
  port: 5555
  tls_cert: cert.crt
  tls_key: cert.key
  base_url: https://cloud.padlock.io
  cors: false
leveldb:
  path: path/to/db
email:
  server: smtp.gmail.com
  port: "587"
  user: mail_user
  password: secret
  from: mail@example.com
log:
  log_file: LOG.txt
  err_file: ERR.txt
  notify_errors: admin@example.com
```

## Docker
[![Docker Build Status](https://img.shields.io/docker/build/padlock/padlock-cloud.svg?style=flat-square)](https://hub.docker.com/r/padlock/padlock-cloud/)
[![Docker Automated Build](https://img.shields.io/docker/automated/padlock/padlock-cloud.svg?style=flat-square)](https://hub.docker.com/r/padlock/padlock-cloud/)
[![Docker Pulls](https://img.shields.io/docker/pulls/padlock/padlock-cloud.svg?style=flat-square)](https://hub.docker.com/r/padlock/padlock-cloud/)
[![Docker Stars](https://img.shields.io/docker/stars/padlock/padlock-cloud.svg?style=flat-square)](https://hub.docker.com/r/padlock/padlock-cloud/)

### Getting Started with Docker
**NOTE**: As padlock is build upon chrome we need a valid certificate issued
by a trusted source. For now let us assume we have a certificate named
`cert.pem` and a key named `key.pem` in the directory `./ssl/`.

**NOTE**: The email-settings can be found out by searching for 
`[email-provider] smtp login`.

```sh
docker run -p 443:8443 -v ssl:/opt/padlock-cloud/ssl -e PC_PORT=8443 \
-e PC_BASE_URL=[base-url] -e PC_EMAIL_SERVER=smtp.googlemail.com \
-e PC_EMAIL_PORT=587 -e PC_EMAIL_USER=user@gmail.com \
-e PC_EMAIL_PASSWORD=userpassword1234 \
-e PC_TLS_CERT=/opt/padlock-cloud/ssl/cert.pem \
-e PC_TLS_KEY=/opt/padlock-cloud/ssl/key.pem padlock/padlock-cloud
```

### Usage with Docker
This image can be used like the cli. Just prepend `docker run`.

### Volumes
**NOTE**: This image uses a user `padlock-cloud` with uid `1000` and group 
`padlock-cloud` with gid `1000` to run padlock-cloud. You should check your 
permission before mounting a volume.  
**NOTE**: This image will try to change the ownership of it's WORKDIR to 
`1000:1000`. This won't work when mounting a volume from Windows.  

This image contains 4 volumes.
#### /opt/padlock-cloud/assets
Contains assets used by padlock-cloud to render the frontend and the emails.

#### /opt/padlock-cloud/db
Contains the data stored in the cloud.

#### /opt/padlock-cloud/logs
Contains the logs.

#### /opt/padlock-cloud/ssl
Contains the certificate and key.

### Bindings
This image exposes ports `8080` and `8443`, because this image uses a non-root
user. By default the padlock-cloud listens at port `8080`, because it doesn't
use SSL by default. It is highly suggested to provide a TLS-Certificate and
Key to enable SSL and listen at `8443`. This could be done by setting `PC_PORT`
to `8443`.

### Security
This image uses a user `padlock-cloud` with uid `1000` and group 
`padlock-cloud` with gid `1000` to run padlock-cloud.  
**It will try to change the ownership of your mounted volumes to 
`1000:1000`.**  

## How to install/build

First, you'll need to have [Go](https://golang.org/) installed on your system.
Then simply run

```sh
go get github.com/padlock/padlock-cloud
```

This will download the source code into your `$GOPATH` and automatically build
and install the `padlock-cloud` binary into `$GOPATH/bin`. Assuming you have
`$GOPATH/bin` added to your path, you should be the be able to simply run the
`padlock-cloud` command from anywhere.

## Security Considerations

### Running the server without TLS

It goes without saying that user data should **never** be transmitted over the
internet over a non-secure connection. If no `--tls-cert` and `--tls-key`
options are provided to the `runserver` command, the server will be addressable
through plain http. You should make sure that in this case the server does
**not** listen on a public port and that any reverse proxies that handle
outgoing connections are protected via TLS.

### Link spoofing and the --base-url option

Padlock Cloud frequently uses confirmation links for things like activating
authentication tokens, confirmation for deleting an account etc. They usually
contain some sort of unique token. For example, the link for activating an
authentication token looks like this:

```
https://hostname:port/activate/?v=1&t=cdB6iEdL4o5PfhLey30Rrg
```

These links are sent out to a users email address and serve as a form of
authentication. Only users that actually have control over the email account
associated with their Padlock Cloud account may access the corresponding data.

Now the `hostname` and `port` portion of the URL will obviously differ based on
the environment. By default, the app will simply use the value provided by the
`Host` header of the incoming request. But the `Host` header can easily be
faked and unless the server is running behind a reverse proxy that sets it
to the correct value, this opens the app up to a vulnerability we call 'link
spoofing'. Let's say an attacker sends an authentication request to our server
using a targets email address, but changes the `Host` header to a server that
he or she controls. The email that is sent to the target will now contain a link that
points to the attacker's server instead of our own and once the user clicks the
link the attacker is in possession of the activation token which can in turn be
used to activate the authentication token he or she already has.  There is a simple
solution for this: Explicitly provide a base URL to be used for constructing
links when starting up the server. The `runserver` command provides the
`--base-url` flag for this. It is recommended to use this option in production
environments at all times!

## Troubleshooting

### Chrome app fails to connect to custom server

When trying to connect to a custom server instance, the Chrome app fails with
the error message "Failed to connect to Padlock Cloud. Please check your
internet and try again!".  This is due to the same-origin policy in Chrome
preventing requests to domains other than cloud.padlock.io that do not
implement [Cross-Origin Resource
Sharing](https://developer.mozilla.org/en-US/docs/Web/HTTP/Access_control_CORS).
While it's not enabled by default, Padlock Cloud does come with built-in CORS
support. In order to enable it, simple use the `cors` option. E.g.:

```sh
padlock-cloud runserver --cors
```

**NOTE**: CORS is enabled by default when using the [docker image](#docker).

### Failed to load templates

```sh
2016/09/01 21:40:59 open some/path/activate-auth-token-email.txt: no such file or directory
```

The Padlock Cloud server requires various assets like templates for rendering
emails, web pages etc. These are included in this repository under the `assets`
folder. When you're running `padlock-cloud` you'll have to make sure that it
knows where to find these assets. You can do this via the `--assets-path`
option. By default, the server will look for the templates under
`$GOPATH/src/github.com/padlock/padlock-cloud/assets/templates` which is
where they will usually be if you installed `padlock-cloud` via `go get`.
