#!/usr/bin/env python3
"""Экспортер бизнес-метрик workflow release notes для Prometheus.

n8n в /metrics отдаёт только технический итог запуска (success / error).
Этот экспортер читает историю запусков через публичный API n8n и разбирает,
чем закончился каждый запуск с точки зрения процесса:

  draft_created   — агент написал заметки, черновик Release создан;
  release_exists  — для нового тега Release уже есть, делать нечего;
  rejected        — ответ агента не прошёл проверку (Reject Agent Output);
  failed_github   — упала HTTP- или GitHub-нода (все HTTP-ноды workflow ходят в GitHub API);
  failed_ai       — упал агент, модель или MCP-инструмент;
  failed_other    — всё остальное (Code-нода, отмена, падение процесса n8n).

Метрики двух видов:
  *_total               — счётчики запусков, завершившихся после старта экспортера.
                          Все значения меток заведены заранее с нулём, поэтому первое
                          событие видно в increase(). Нужны для графиков.
  release_notes_last_*  — состояние, вычисленное из истории запусков в базе n8n.
                          Переживает перезапуск n8n и экспортера. Нужно для алертов.
                          Появляется только после первого успешного опроса API:
                          «неизвестно» не выдаётся за ноль.

Переменные окружения:
  N8N_API_KEY      — API-ключ n8n (Settings → n8n API), обязательно;
  N8N_WORKFLOW_ID  — id workflow (из URL редактора), обязательно;
  N8N_URL          — адрес n8n, по умолчанию http://n8n:5678;
  POLL_SECONDS     — период опроса API, по умолчанию 30;
  PORT             — порт /metrics, по умолчанию 9800.
"""

import json
import logging
import os
import time
import urllib.parse
import urllib.request
from datetime import datetime

from prometheus_client import REGISTRY, Counter, Gauge, disable_created_metrics, start_http_server

N8N_URL = os.environ.get("N8N_URL", "http://n8n:5678").rstrip("/")
API_KEY = os.environ["N8N_API_KEY"]
WORKFLOW_ID = os.environ["N8N_WORKFLOW_ID"]
POLL_SECONDS = int(os.environ.get("POLL_SECONDS", "30"))
PORT = int(os.environ.get("PORT", "9800"))

RESULTS = ("draft_created", "release_exists", "rejected", "failed_github", "failed_ai", "failed_other")
FINISHED = {"success", "error", "crashed", "canceled"}

AGENT_TYPE = "@n8n/n8n-nodes-langchain.agent"
LLM_TYPE_PREFIX = "@n8n/n8n-nodes-langchain.lmChat"
LANGCHAIN_PREFIX = "@n8n/n8n-nodes-langchain."
GITHUB_TYPES = {"n8n-nodes-base.httpRequest", "n8n-nodes-base.github"}
REJECT_TYPE = "n8n-nodes-base.stopAndError"

disable_created_metrics()  # без служебных рядов *_created

EXECUTIONS = Counter(
    "release_notes_executions",
    "Завершённые запуски workflow по итогу для процесса",
    ["result"],
)
LLM_CALLS = Counter("release_notes_llm_calls", "Вызовы языковой модели")
LLM_TOKENS = Counter("release_notes_llm_tokens", "Токены языковой модели", ["type"])

# registry=None: регистрируются в poll() после первого успешного опроса API.
LAST_RESULT = Gauge(
    "release_notes_last_execution_result",
    "Итог последнего завершённого запуска: 1 у текущего итога, 0 у остальных",
    ["result"],
    registry=None,
)
LAST_TIMESTAMP = Gauge(
    "release_notes_last_execution_timestamp_seconds",
    "Время окончания последнего завершённого запуска (unix time)",
    registry=None,
)
LAST_SUCCESS = Gauge(
    "release_notes_last_success_timestamp_seconds",
    "Время окончания последнего успешного запуска (unix time); 0 — успешных не было",
    registry=None,
)
STATE_METRICS = (LAST_RESULT, LAST_TIMESTAMP, LAST_SUCCESS)
SOURCE_UP = Gauge("release_notes_source_up", "1 — последний опрос API n8n прошёл успешно")

for result in RESULTS:
    EXECUTIONS.labels(result=result)
    LAST_RESULT.labels(result=result)
for kind in ("prompt", "completion"):
    LLM_TOKENS.labels(type=kind)


def api(path, **params):
    query = urllib.parse.urlencode(params)
    request = urllib.request.Request(
        f"{N8N_URL}/api/v1{path}?{query}",
        headers={"X-N8N-API-KEY": API_KEY, "Accept": "application/json"},
    )
    with urllib.request.urlopen(request, timeout=20) as response:
        return json.load(response)


def list_finished(**params):
    """Завершённые запуски workflow, от новых к старым."""
    data = api("/executions", workflowId=WORKFLOW_ID, **params)["data"]
    return [e for e in data if e["status"] in FINISHED]


def timestamp(value):
    return datetime.fromisoformat(value.replace("Z", "+00:00")).timestamp()


def node_types(execution):
    nodes = (execution.get("workflowData") or {}).get("nodes", [])
    return {n["name"]: n for n in nodes}


def classify(execution):
    nodes = node_types(execution)
    result_data = (execution.get("data") or {}).get("resultData", {})
    ran = set(result_data.get("runData", {}))

    if execution["status"] == "success":
        for name in ran:
            node = nodes.get(name, {})
            params = node.get("parameters", {})
            if node.get("type") == "n8n-nodes-base.github" and params.get("operation") == "create":
                return "draft_created"
        # Старая версия workflow: ветка false после проверки заканчивалась без ошибки.
        if any(nodes.get(name, {}).get("type") == AGENT_TYPE for name in ran):
            return "rejected"
        return "release_exists"

    error = result_data.get("error") or {}
    failed_type = (error.get("node") or {}).get("type") or nodes.get(
        result_data.get("lastNodeExecuted", ""), {}
    ).get("type", "")
    if failed_type == REJECT_TYPE:
        return "rejected"
    if failed_type in GITHUB_TYPES:
        return "failed_github"
    if failed_type.startswith(LANGCHAIN_PREFIX):
        return "failed_ai"
    return "failed_other"


def llm_usage(execution):
    nodes = node_types(execution)
    run_data = (execution.get("data") or {}).get("resultData", {}).get("runData", {})
    calls = prompt = completion = 0
    for name, runs in run_data.items():
        if not nodes.get(name, {}).get("type", "").startswith(LLM_TYPE_PREFIX):
            continue
        for run in runs:
            calls += 1
            for output in (run.get("data") or {}).get("ai_languageModel", []):
                for item in output or []:
                    usage = item.get("json", {})
                    usage = usage.get("tokenUsage") or usage.get("tokenUsageEstimate") or {}
                    prompt += usage.get("promptTokens", 0)
                    completion += usage.get("completionTokens", 0)
    return calls, prompt, completion


class Poller:
    def __init__(self):
        self.initialized = False
        self.seen = set()
        self.results = {}  # id запуска → итог; запуск после окончания не меняется

    def result_of(self, execution_id):
        if execution_id not in self.results:
            full = api(f"/executions/{execution_id}", includeData="true")
            self.results[execution_id] = (classify(full), llm_usage(full))
        return self.results[execution_id]

    def poll(self):
        finished = list_finished(limit=50)

        # При первом опросе история только запоминается: счётчики считают новые запуски.
        for execution in reversed(finished):
            if execution["id"] in self.seen:
                continue
            if self.initialized:
                result, (calls, prompt, completion) = self.result_of(execution["id"])
                EXECUTIONS.labels(result=result).inc()
                LLM_CALLS.inc(calls)
                LLM_TOKENS.labels(type="prompt").inc(prompt)
                LLM_TOKENS.labels(type="completion").inc(completion)
                logging.info(
                    "execution %s: %s, llm calls %d, tokens %d/%d",
                    execution["id"], result, calls, prompt, completion,
                )
            self.seen.add(execution["id"])

        if finished:
            last = finished[0]
            result, _ = self.result_of(last["id"])
            for name in RESULTS:
                LAST_RESULT.labels(result=name).set(1 if name == result else 0)
            LAST_TIMESTAMP.set(timestamp(last["stoppedAt"]))

        last_success = list_finished(status="success", limit=1)
        LAST_SUCCESS.set(timestamp(last_success[0]["stoppedAt"]) if last_success else 0)

        # Старые id уже не вернутся в выборку из 50 последних — память не растёт.
        current = {e["id"] for e in finished}
        self.seen &= current
        self.results = {k: v for k, v in self.results.items() if k in current}
        if not self.initialized:
            for metric in STATE_METRICS:
                REGISTRY.register(metric)
        self.initialized = True


def main():
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    start_http_server(PORT)
    logging.info("listening on :%d, workflow %s, n8n %s", PORT, WORKFLOW_ID, N8N_URL)
    poller = Poller()
    while True:
        try:
            poller.poll()
            SOURCE_UP.set(1)
        except (OSError, KeyError, ValueError) as exc:  # сеть, HTTP-ошибки, неожиданный ответ
            SOURCE_UP.set(0)
            logging.warning("n8n API poll failed: %s", exc)
        time.sleep(POLL_SECONDS)


if __name__ == "__main__":
    main()
