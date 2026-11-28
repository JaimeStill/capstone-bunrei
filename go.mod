module github.com/JaimeStill/capstone-bunrei

go 1.27.4

require (
	github.com/JaimeStill/capstone-bunrei/client v0.0.0-00010101000000-000000000000
	github.com/standards-lab/docinfer v0.5.0
	github.com/standards-lab/docinfer/postgres v0.1.3
	github.com/standards-lab/docproc v0.10.0
	github.com/standards-lab/docproc/postgres v0.1.8
	github.com/standards-lab/go-core v0.13.0
	github.com/standards-lab/go-database v0.13.0
	github.com/standards-lab/go-database/postgres v0.7.0
	github.com/standards-lab/go-messaging v0.6.0
	github.com/standards-lab/go-messaging/postgres v0.5.0
	github.com/standards-lab/go-web-sdk v0.19.0
	github.com/standards-lab/sqlate v0.8.0
	github.com/standards-lab/sqlate/postgres v0.9.0
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.13.0 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	github.com/standards-lab/go-ai v0.5.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/text v0.44.0 // indirect
)

replace github.com/JaimeStill/capstone-bunrei/client => ./client
