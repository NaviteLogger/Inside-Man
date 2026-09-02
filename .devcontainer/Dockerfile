FROM mcr.microsoft.com/devcontainers/python:latest@sha256:97152d71a6e42cc2f4f39d68fe72a94fc10baeb203700296dd3b2db4cfaaa686

ARG NODE_VERSION=24
ARG OPENTOFU_VERSION=1.12.6
ARG GITLEAKS_VERSION=8.30.1

LABEL org.opencontainers.image.source="https://github.com/jhoareaumarion/devcontainers"
LABEL org.opencontainers.image.description="Inside Man Devcontainer"
LABEL org.opencontainers.image.licenses="MPL 2.0"

USER root

RUN apt-get update && apt-get upgrade -y

RUN apt-get update && \
    export DEBIAN_FRONTEND=noninteractive && \
    apt-get install -y --no-install-recommends \
        ansible-core \
        ansible-lint \
        libdbd-pg-perl \
        postgresql-client \
        sqitch \
        zsh && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /workspaces/Inside-Man

USER vscode
RUN bash -c "source /usr/local/share/nvm/nvm.sh \
    && nvm install ${NODE_VERSION} \
    && nvm alias default ${NODE_VERSION} \
    && npm install -g @juliusbrussee/caveman-code @openai/codex bun@1.4.0"
ENV PATH="/home/vscode/.bun/bin:$PATH"

CMD ["/bin/bash"]
