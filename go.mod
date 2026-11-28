module github.com/JaimeStill/capstone-bunrei

go 1.27.4

require (
	github.com/JaimeStill/capstone-bunrei/client v0.0.0-00010101000000-000000000000
	github.com/standards-lab/docinfer v0.5.0
	github.com/standards-lab/docproc v0.10.0
	github.com/standards-lab/go-core v0.13.0
	github.com/standards-lab/go-messaging v0.5.0
	github.com/standards-lab/go-web-sdk v0.19.0
)

require (
	github.com/standards-lab/go-ai v0.5.0 // indirect
	github.com/standards-lab/sqlate v0.8.0 // indirect
	golang.org/x/text v0.44.0 // indirect
)

replace github.com/JaimeStill/capstone-bunrei/client => ./client
