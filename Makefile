.PHONY=deps
deps:
	docker-compose up -d

.PHONY=up
up: deps
	go run .../.

.PHONY=down
down:
	docker-compose down

