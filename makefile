
all:cli lam server sendfile

cli:fmt
	export GO111MODULE=on && go vet --tags "cli"
	export GOOS=linux && export GO111MODULE=on && go build --tags "cli" -o det_cli

lam:fmt
	export GO111MODULE=on && go vet --tags "lambda"
	export GOOS=linux && export GO111MODULE=on && go build --tags "lambda" -o det_lam

server:fmt
	export GO111MODULE=on && go vet --tags "server"
	export GOOS=linux && export GO111MODULE=on && go build --tags "server" -o det_server

sendfile:fmt
	export GO111MODULE=on && go vet --tags "sendfile"
	export GOOS=linux && export GO111MODULE=on && go build --tags "sendfile" -o sendfile


fmt:
	go fmt ./...

zip:lam
	zip main.zip det_lam

clean:
	rm -rf main.zip det_cli det_lam det_server sendfile

