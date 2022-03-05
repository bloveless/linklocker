.PHONY=deps
deps:
	docker-compose up -d

.PHONY=up
up: deps
	SESSION_KEY=1cawJhUYlziJgjRBva47jBy1NizU69Jb8I7JS04c go run .../.

.PHONY=down
down:
	docker-compose down

