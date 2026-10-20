FROM node:24.22-alpine

RUN npm install -g \
    supergateway@4.2.0 \
    @modelcontextprotocol/server-filesystem@1.0.0

USER node

CMD ["supergateway", "--stdio", "mcp-server-filesystem /docs", "--outputTransport", "streamableHttp", "--port", "8000", "--healthEndpoint", "/healthz"]
