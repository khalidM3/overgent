module github.com/stickguy/stickguy/validation/spikes/multilang-contract

go 1.26.0

require golang.org/x/sys v0.47.0 // indirect

require (
	github.com/stickguy/stickguy v0.0.0
	github.com/tetratelabs/wazero v1.12.0
)

replace github.com/stickguy/stickguy => ../../..
