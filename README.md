# Padlock Cloud

Padlock Cloud is a cloud storage and synchronization service for the
[Padlock app](https://github.com/MaKleSoft/padlock/) implemented in Go. It provides a (mostly) RESTful api
for storing and retrieving a users data. Users are identified by their email addresses, which are in most
cases provided as part of the authentication credentials. Padlock Cloud does NOT implement any kind of
diffing algorithm, nor does it attempt at providing any kind of cryptographic functionality. Any encryption,
decryption and data consolidation should happen on the client side. Padlock Cloud merely provides a convenient
way of storing encrypted dumps of user data in the cloud.

**Note**: This is a library package for use in other Go applications. If you want a runnable binary, take a
look at [padlock-cloud-server](https://github.com/maklesoft/padlock-cloud-server).

## Usage

```sh
go get github.com/maklesoft/padlock-cloud
```

```go
import "github.com/maklesoft/padlock-cloud"
```
