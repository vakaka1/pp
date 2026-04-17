module github.com/user/pp

go 1.22

require (
	github.com/bits-and-blooms/bloom/v3 v3.7.0
	github.com/flynn/noise v1.1.0
	github.com/golang-jwt/jwt/v5 v5.2.1
	github.com/mattn/go-sqlite3 v1.14.42
	github.com/oschwald/maxminddb-golang v1.13.0
	github.com/refraction-networking/utls v1.6.4
	github.com/spf13/cobra v1.10.2
	github.com/xtaci/smux v1.5.57
	go.uber.org/zap v1.27.1
	golang.org/x/crypto v0.22.0
	golang.org/x/net v0.21.0
	golang.org/x/sys v0.20.0
)

require (
	github.com/andybalholm/brotli v1.0.6 // indirect
	github.com/bits-and-blooms/bitset v1.10.0 // indirect
	github.com/cloudflare/circl v1.3.7 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/compress v1.17.4 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	golang.org/x/text v0.14.0 // indirect
)

replace github.com/mattn/go-sqlite3 => ./third_party/go-sqlite3
