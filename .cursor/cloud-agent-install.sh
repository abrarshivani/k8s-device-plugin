#!/usr/bin/env bash
# Copyright (c) 2026 NVIDIA CORPORATION
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

GOLANG_VERSION="$(./hack/golang-version.sh)"

install_go() {
	if command -v go >/dev/null 2>&1 && go version | grep -q "go${GOLANG_VERSION} "; then
		return 0
	fi
	curl -fsSL "https://go.dev/dl/go${GOLANG_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tar.gz
	sudo rm -rf /usr/local/go
	sudo tar -C /usr/local -xzf /tmp/go.tar.gz
	sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go
	sudo ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
}

install_build_deps() {
	if command -v gcc >/dev/null 2>&1; then
		return 0
	fi
	sudo apt-get update -qq
	sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq build-essential pkg-config
}

link_tool() {
	local name="$1"
	if [[ -x /usr/local/bin/"$name" ]]; then
		return 0
	fi
	sudo ln -sf "${HOME}/go/bin/${name}" "/usr/local/bin/${name}"
}

install_go_tool() {
	local name="$1"
	local pkg="$2"
	if command -v "$name" >/dev/null 2>&1; then
		return 0
	fi
	mkdir -p "${HOME}/go/bin"
	GOBIN="${HOME}/go/bin" go install "$pkg"
	link_tool "$name"
}

install_go
export PATH="/usr/local/go/bin:/usr/local/bin:${HOME}/go/bin:${PATH}"
export CGO_ENABLED=1

install_build_deps

install_go_tool golangci-lint github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
install_go_tool moq github.com/matryer/moq@latest
install_go_tool helm helm.sh/helm/v3/cmd/helm@v3.16.4

go mod download
