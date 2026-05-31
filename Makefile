.PHONY=deps
deps:
	docker-compose up -d

.PHONY=up
up: deps
	gow -e=go,tmpl run ./... main.go

.PHONY=down
down:
	docker-compose down

