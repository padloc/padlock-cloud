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
    ```
    git clone https://github.com/MaKleSoft/padlock-cloud.git
    ```
3. Build binary
    ```
    cd padlock-cloud
    go build
    ```
4. Run it!
    ```
    ./padlock-cloud
    ```
