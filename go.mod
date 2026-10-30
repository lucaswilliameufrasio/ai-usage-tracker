module ai-usage-tracker

go 1.27.3

require (
	github.com/SherClockHolmes/webpush-go v1.4.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/lucaswilliameufrasio/sqlite-replication v0.0.0
	github.com/mattn/go-isatty v0.0.24
	modernc.org/sqlite v1.61.0
)

replace github.com/lucaswilliameufrasio/sqlite-replication => ../sqlite-replication/go

require (
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	modernc.org/libc v1.79.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)
