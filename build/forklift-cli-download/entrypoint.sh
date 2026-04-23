#!/bin/sh

kubectl-mtv mcp-server --http \
  --host "" --port 8081 \
  --output-format "${MCP_OUTPUT_FORMAT:-markdown}" \
  ${MCP_MAX_RESPONSE_CHARS:+--max-response-chars "$MCP_MAX_RESPONSE_CHARS"} \
  ${MCP_KUBE_SERVER:+--server "$MCP_KUBE_SERVER"} \
  ${MCP_KUBE_TOKEN:+--token "$MCP_KUBE_TOKEN"} \
  $([ "${MCP_KUBE_INSECURE}" = "true" ] && echo --insecure-skip-tls-verify) \
  $([ "${MCP_READ_ONLY}" = "true" ] && echo --read-only) \
  ${MCP_VERBOSE:+--verbose "$MCP_VERBOSE"} &
MCP_PID=$!

(while kill -0 "$MCP_PID" 2>/dev/null; do sleep 5; done; echo "mcp-server (PID $MCP_PID) exited unexpectedly" >&2; kill 1) &

exec nginx -g "daemon off;" -c /opt/app-root/etc/nginx.d/nginx.conf
