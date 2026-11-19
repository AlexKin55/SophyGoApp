.PHONY: run build test vet fmt tidy clean

# Запуск сервиса для локальной разработки.
# Если рядом лежит .env, переменные загружаются в окружение процесса.
# Значения с пробелами должны быть в кавычках: KEY="значение с пробелами".
run:
	@set -a; [ ! -f .env ] || . ./.env; set +a; go run ./cmd/server

# Сборка бинарника в bin/sophygo-api.
build:
	go build -o bin/sophygo-api ./cmd/server

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy

clean:
	rm -rf bin
