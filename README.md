# Run tests and GraphQL Server
`go clean -testcache`
`go test ./oas/... -v`
`go run .`

## To do

- add schema snapshot for tests
- support multipart/form-data
- support oas links (nested resolvers)
- headers params
- support security schemas
- subscriptions
