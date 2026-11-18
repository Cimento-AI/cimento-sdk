module github.com/cimento-ai/cimento-sdk/internal

go 1.26

require (
	github.com/cimento-ai/cimento-sdk/go/webhooks v0.0.0
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3
)

require golang.org/x/text v0.14.0 // indirect

// The SDK this module's tooling checks is the one in this repository.
replace github.com/cimento-ai/cimento-sdk/go/webhooks => ../go/webhooks
