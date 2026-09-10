# Mailinator

Mailinator is a web service for checking email sent to public, temporary email addresses.

Mailinator stores the mailboxes and messages in-memory in a concurrency safe way. The web service
provides cursor-based pagination and an eviction strategy configurable through command-line flags.

## Installation

Build the `mailinator` binary with:

```
$ go build ./...
```

## Usage

Run the server directly with `go run`:

```
$ go run . -port 8080
```

Or run the built binary:

```
$ ./mailinator -h
Usage of mailinator:
  -eviction-interval duration
        Message eviction interval (default 5m0s)
  -message-ttl duration
        Message time to live (default 2h0m0s)
  -port int
        API server port (default 8080)
```

## Testing

Run the full test suite with:

```
$ go test -race ./...
```

## API endpoints

Mailinator expose the following HTTP endpoints.

- `POST /mailboxes`: Create a new, random email address.
- `POST /mailboxes/{email address}/messages`: Create a new message for a specific email address.
- `GET /mailboxes/{email address}/messages`: Retrieve an index of messages sent to an email address, including sender, subject, and id, in recency order. Support cursor-based pagination through the index.
- `GET /mailboxes/{email address}/messages/{message id}`: Retrieve a specific message by id.
- `DELETE /mailboxes/{email address}`: Delete a specific email address and any associated messages.
- `DELETE /mailboxes/{email address}/messages/{message id}`: Delete a specific message by id.
