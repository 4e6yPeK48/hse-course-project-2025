.PHONY: up down logs

up:
	docker compose -f app/docker-compose.yml up --build -d --wait

down:
	docker compose -f app/docker-compose.yml down

logs:
	docker compose -f app/docker-compose.yml logs -f
