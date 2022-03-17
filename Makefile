.PHONY=deps
deps:
	docker-compose up -d

.PHONY=up
up: deps
	SESSION_SECRET=1cawJhUYlziJgjRBva47jBy1NizU69Jb8I7JS04c CSRF_SECRET=8Ch8TU78Qi3mtN4EWlQiPkKdYTm4RM6Gydr9Bc2S go run .../.

.PHONY=down
down:
	docker-compose down

