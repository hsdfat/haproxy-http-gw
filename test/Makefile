.PHONY: help setup certs up down logs test test-functional test-perf test-quick clean reset

help:
	@echo "HTTP Gateway Test System"
	@echo ""
	@echo "Available targets:"
	@echo "  setup           - Complete setup (generate certs, build, start)"
	@echo "  certs           - Generate SSL certificates"
	@echo "  up              - Start all services"
	@echo "  down            - Stop all services"
	@echo "  logs            - View logs from all services"
	@echo "  test            - Run all tests"
	@echo "  test-functional - Run functional tests only"
	@echo "  test-perf       - Run performance tests only"
	@echo "  test-quick      - Run quick smoke test"
	@echo "  clean           - Stop services and remove volumes"
	@echo "  reset           - Complete cleanup and rebuild"

certs:
	@echo "Generating SSL certificates..."
	./scripts/generate-certs.sh

up:
	@echo "Starting test environment..."
	docker-compose up -d
	@echo "Waiting for services to be ready..."
	@sleep 10
	@docker-compose ps

down:
	@echo "Stopping test environment..."
	docker-compose down

logs:
	docker-compose logs -f

setup: certs
	@echo "Building and starting test environment..."
	docker-compose up -d --build
	@echo "Waiting for services to be ready..."
	@sleep 15
	@echo ""
	@echo "Test environment is ready!"
	@echo ""
	@echo "Services:"
	@docker-compose ps
	@echo ""
	@echo "To run tests: make test"

test: test-functional test-perf

test-functional:
	@echo "Running functional tests..."
	docker-compose run --rm test-client /test-client -gateway=http://gateway:8080 -gateway-https=https://gateway:8443 -verbose

test-perf:
	@echo "Running performance tests..."
	@echo ""
	@echo "Test 1: Low concurrency (10 workers, 1000 requests)"
	docker-compose run --rm test-client /perf-client -url=http://gateway:8080 -c=10 -n=1000
	@echo ""
	@echo "Test 2: Medium concurrency (50 workers, 5000 requests)"
	docker-compose run --rm test-client /perf-client -url=http://gateway:8080 -c=50 -n=5000
	@echo ""
	@echo "Test 3: High concurrency (100 workers, 10000 requests)"
	docker-compose run --rm test-client /perf-client -url=http://gateway:8080 -c=100 -n=10000

test-quick:
	@echo "Running quick smoke test..."
	docker-compose run --rm test-client /test-client -gateway=http://gateway:8080
	docker-compose run --rm test-client /perf-client -url=http://gateway:8080 -c=10 -n=100

clean:
	@echo "Cleaning up..."
	docker-compose down -v

reset: clean
	@echo "Removing images..."
	docker-compose down -v --rmi local
	@echo "Rebuilding..."
	$(MAKE) setup
