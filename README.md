# Padlock Cloud

Padlock Cloud is a cloud storage and synchronization service for the
[Padlock app](https://github.com/maklesoft/padlock/) implemented in Go. It provides a (mostly) RESTful api
for storing and retrieving user data. Padlock Cloud does NOT implement any kind of
diffing algorithm, nor does it attempt to provide any kind of cryptographic functionality. Any encryption,
decryption and data consolidation should happen on the client side. Padlock Cloud merely provides a
cloud-based storage for encrypted user data.

## How to install/build

First, you'll need to have [Go](https://golang.org/) installed on your system. Then simply run

```sh
go get github.com/maklesoft/padlock-cloud
```

This will download the source code into your `$GOPATH` and automatically build and install the
`padlock-cloud` binary into `$GOPATH/bin`. Assuming you have `$GOPATH/bin` added
to your path, you should be the be able to simply run the `padlock-cloud` command from anywhere.

## Usage

The `padlock-cloud` command provides commands for starting Padlock Cloud server and managing
accounts. It can be configured through various flags and environment variables.

Note that **global flags** have to be specified **before** the command and **command-specific** flags
**after** the command but before any positional arguments.

```sh
padlock-cloud [global options] command [command options] [arguments...]
```

For a list of available commands and global options, run.

```sh
padlock-cloud --help
```

For information about a specific command, including command-specific options, run

```sh
padlock-cloud command --help
```

### Config file

The `--config` flag offers the option of using a configuration file instead of command line flags. The
provided file should be in the [YAML format]http://yaml.org/). Here is an example configuration file:

```yaml
---
server:
  require_tls: true
  assets_path: assets
  port: 5555
  tls_cert: cert.crt
  tls_key: cert.key
  host_name: cloud.padlock.io
leveldb:
  path: path/to/db
email:
  server: smtp.gmail.com
  port : "587"
  user: mail@example.com
  password: secret
log:
  log_file: LOG.txt
  err_file: ERR.txt
  notify_errors: admin@example.com
```

## Troubleshooting

### Failed to load templates

```sh
2016/09/01 21:40:59 open asdf/templates/activate-auth-token-email.txt: no such file or directory
```

The Padlock Cloud server requires various assets like templates for rendering emails, web
pages etc. These are included in this repository under the `assets` folder. When you're running
`padlock-cloud` you'll have to make sure that it knows where to find these assets. You can do that
via the `--assets-path` option. By default, the server will look for the templates under
`$GOPATH/src/github.com/maklesoft/padlock-cloud/assets/templates` which is where they will usually be
if you installed `padlock-cloud` via `go get`.
