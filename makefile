# ==== Config ====
MODULE := github.com/LeonardoBeccarini/sdcc_project
PROTO_DIR := deploy/proto
GEN_DIR := deploy/gen/go

PROTOS := $(PROTO_DIR)/irrigation.proto

# ==== Tools ====
.PHONY: tools
tools:
	@echo "→ Installing protoc plugins (go/protobuf + go-grpc)"
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.2
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.4.0

# ==== Codegen ====
.PHONY: proto
proto:
	@echo "→ Genero Go + gRPC in $(GEN_DIR)"
	@mkdir -p $(GEN_DIR)
	@protoc -I $(PROTO_DIR) \
		--go_out=$(GEN_DIR) --go_opt=module=$(MODULE) \
		--go-grpc_out=$(GEN_DIR) --go-grpc_opt=module=$(MODULE) \
		$(PROTOS)
	@echo "✓ Fatto. File generati sotto $(GEN_DIR)"

.PHONY: clean
clean:
	@echo "→ Rimuovo $(GEN_DIR)"
	@rm -rf $(GEN_DIR)
	@echo "✓ Pulizia completata"

# ==== Docker build (minikube o locale) ====
.PHONY: build-device build-irrigation
build-device: proto
	docker build -t deviceservice:latest -f internal/services/device/Dockerfile .

build-irrigation: proto
	docker build -t irrigationcontroller:latest -f internal/services/irrigation-controller/Dockerfile .
