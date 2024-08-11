include .env

create_migration:
	migrate create -ext=sql -dir=internal/store/pgstore/migrations -seq ${name}

migrate_up:
	migrate -path=internal/store/pgstore/migrations -database "postgresql://${DATABASE_USER}:${DATABASE_PASSWORD}@${DATABASE_HOST}:${DATABASE_PORT}/${DATABASE_NAME}?sslmode=disable" -verbose up

migrate_down:
	migrate -path=internal/store/pgstore/migrations -database "postgresql://${DATABASE_USER}:${DATABASE_PASSWORD}@${DATABASE_HOST}:${DATABASE_PORT}/${DATABASE_NAME}?sslmode=disable" -verbose drop

sqlc_generate:
	sqlc generate -f ./internal/store/pgstore/sqlc.yml

.PHONY: create_migration migrate_up migrate_down
