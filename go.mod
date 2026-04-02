module hegel.dev/go/hegel

// todo check the lowest version we can get away with here?
go 1.25.0

require (
	github.com/fxamacker/cbor/v2 v2.9.0
	golang.org/x/exp v0.0.0-20260218203240-3dfff04db8fa
)

require (
	github.com/BurntSushi/toml v1.4.1-0.20240526193622-a339e1f7089c // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/exp/typeparams v0.0.0-20231108232855-2478ac86f678 // indirect
	golang.org/x/mod v0.33.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/tools v0.42.0 // indirect
	honnef.co/go/tools v0.7.0 // indirect
)

tool honnef.co/go/tools/cmd/staticcheck
