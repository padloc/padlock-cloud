# Padlock Cloud

Padlock Cloud is a cloud storage service for the
[Padlock app](https://github.com/maklesoft/padlock/) implemented in Go. It
provides a (mostly) RESTful api for storing and retrieving user data. Padlock
Cloud does NOT implement any kind of diffing algorithm, nor does it attempt to
provide any kind of cryptographic functionality. Any encryption, decryption and
data consolidation should happen on the client side. Padlock Cloud merely
provides a cloud-based storage for encrypted user data.

## How to install/build

First, you'll need to have [Go](https://golang.org/) installed on your system.
Then simply run

```sh
go get github.com/maklesoft/padlock-cloud
```

This will download the source code into your `$GOPATH` and automatically build
and install the `padlock-cloud` binary into `$GOPATH/bin`. Assuming you have
`$GOPATH/bin` added to your path, you should be the be able to simply run the
`padlock-cloud` command from anywhere.

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

### Config file

The `--config` flag offers the option of using a configuration file instead of
command line flags. The provided file should be in the
[YAML format](http://yaml.org/). Here is an example configuration file:

```yaml
---
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

**NOTE**: If you are using a config file, all other flags and environment
variables will be ignored.

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

### Failed to load templates

```sh
2016/09/01 21:40:59 open some/path/activate-auth-token-email.txt: no such file or directory
```

The Padlock Cloud server requires various assets like templates for rendering
emails, web pages etc. These are included in this repository under the `assets`
folder. When you're running `padlock-cloud` you'll have to make sure that it
knows where to find these assets. You can do this via the `--assets-path`
option. By default, the server will look for the templates under
`$GOPATH/src/github.com/maklesoft/padlock-cloud/assets/templates` which is
where they will usually be if you installed `padlock-cloud` via `go get`.
