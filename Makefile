build:
	go build -race -v -o couper main.go

generate:
	go generate main.go

image:
	docker build -t avenga/couper:latest .

test:
	go test -v -short -race -timeout 300s ./...

test-docker:
	docker run --rm -v $(CURDIR):/go/app -w /go/app  golang sh -c "go test -short -count 10 -v -timeout 300s -race ./..."

test-coverage:
	go test -short -timeout 300s -covermode=count -coverprofile=ac.coverage ./accesscontrol
	go test -short -timeout 300s -covermode=count -coverprofile=cache.coverage ./cache
	go test -short -timeout 300s -covermode=count -coverprofile=command.coverage ./command
	go test -short -timeout 300s -covermode=count -coverprofile=config.coverage ./config
	go test -short -timeout 300s -covermode=count -coverprofile=docs.coverage ./docs
	# go test -short -timeout 300s -covermode=count -coverprofile=errors.coverage ./errors
	go test -short -timeout 300s -covermode=count -coverprofile=eval.coverage ./eval
	go test -short -timeout 300s -covermode=count -coverprofile=handler.coverage ./handler
	# go test -short -timeout 300s -covermode=count -coverprofile=logging.coverage ./logging
	go test -short -timeout 300s -covermode=count -coverprofile=server.coverage ./server
	go test -short -timeout 300s -covermode=count -coverprofile=main.coverage ./
	$(MAKE) test-coverage-show

test-coverage-show:
	go tool cover -html=ac.coverage
	go tool cover -html=cache.coverage
	go tool cover -html=command.coverage
	go tool cover -html=config.coverage
	go tool cover -html=docs.coverage
	# go tool cover -html=errors.coverage
	go tool cover -html=eval.coverage
	go tool cover -html=handler.coverage
	# go tool cover -html=logging.coverage
	go tool cover -html=server.coverage
	go tool cover -html=main.coverage
