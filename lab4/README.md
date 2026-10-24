# Лабораторная работа 4 — SLA критического пути под перегрузкой

## Структура проекта

```text
lab4/
├── README.md
├── lab.md
├── kind.yaml
├── infra/
│   ├── local-path-config.yaml
│   └── local-path-provisioner.yaml
├── policies/
│   └── require-non-root.yaml
├── load/
│   └── orders.js
├── krr/
│   ├── cnpg.py
│   ├── guaranteed-report.json
│   ├── run.sh
│   └── report.json
├── batch/
│   ├── .dockerignore
│   ├── Dockerfile
│   ├── go.mod
│   └── main.go
├── helm/
│   ├── monitoring-values.yaml
│   └── shop/
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
└── images/
    ├── part0/
    ├── part2/
    ├── part3/
    ├── part4/
    ├── part5/
    └── part6/
```

---

# Часть 0 — Сервис batch

Создан сервис на Go: CPU-воркеры непрерывно вычисляют SHA-256, а отдельный
цикл удерживает выделенный буфер и каждые 200 мс обращается к каждой странице
памяти. Это обеспечивает реальное заполнение страниц и поддерживает их
активность. Процесс завершает работу по SIGINT/SIGTERM.

| Переменная | По умолчанию | Назначение |
| --- | --- | --- |
| `CPU_WORKERS` | `1` | Число CPU-воркеров; фактическое потребление зависит также от доступного CPU и лимитов контейнера |
| `MEMORY_MIB` | `64` | Размер удерживаемого буфера в MiB; полное потребление процесса несколько выше из-за runtime и служебных данных |

Dockerfile собирает статический бинарный файл и помещает его в образ
`scratch`. Процесс запускается от пользователя `65532:65532`.
Параметры нагрузки при развёртывании будут передаваться из values чарта.

### Проверка контейнера

Образ `lab4-batch:v1` собран из каталога `lab4`:

```bash
docker build -t lab4-batch:v1 ./batch
```

![Успешная сборка образа batch](images/part0/01-batch-build.png)

Контейнер запущен с ограничением 0,5 CPU и 192 MiB памяти, одним
CPU-воркером и буфером 64 MiB:

```bash
docker run -d --name lab4-batch-check \
  --cpus=0.5 --memory=192m --memory-swap=192m \
  -e CPU_WORKERS=1 -e MEMORY_MIB=64 lab4-batch:v1
```

При проверке потребление CPU составило `52.33%`, памяти — `66.9 MiB`.
Потребление памяти немного выше размера буфера из-за накладных расходов
процесса. Контейнер работает от пользователя `65532:65532`,
`running=true`, `oomKilled=false`.

![Запуск batch, потребление ресурсов и пользователь процесса](images/part0/02-batch-running.png)

После `docker stop` процесс записал `batch stopped`, затем контейнер удалён:

![Штатное завершение batch](images/part0/03-batch-stop.png)

**Результат:** сервис создаёт нагрузку на CPU и память, запускается без root
и корректно завершается по SIGTERM. ✅

---

# Часть 1 — Выбор механизма разнесения

Чарт из лабораторной 3 скопирован в `helm/shop`, чтобы изменения лабораторной 4
хранились отдельно. В `values.yaml` добавлен параметр
`scheduling.mechanism: topologySpreadConstraints`; альтернативное значение —
`podAntiAffinity`. Подключение выбранного механизма к шаблону API выполняется
в части 4; сейчас параметр фиксирует выбор и ещё не меняет размещение Pod.

`podAntiAffinity` описывает нежелательное соседство с подходящими по меткам
Pod в одном топологическом домене. Жёсткое правило на уровне hostname
не позволит разместить две реплики API на одной ноде: при трёх репликах
и двух нодах третья останется Pending. Мягкое правило допускает такое
соседство и не гарантирует разнесение.

`topologySpreadConstraints` ограничивает разницу числа подходящих Pod между
топологическими доменами. Для двух доступных нод при `maxSkew: 1` и
`whenUnsatisfiable: DoNotSchedule` три реплики API могут разместиться как
`2+1`, а две — как `1+1`. Поэтому выбран этот механизм: он обеспечивает
баланс и подходит для тесного кластера с тремя исходными репликами API.
При этом ограничения учитывают доступные домены и остальные условия
планирования; они сами по себе не гарантируют работоспособность при потере
ноды и не переносят уже запущенные Pod автоматически.

**Результат:** выбран `topologySpreadConstraints`, выбор зафиксирован
в `values.yaml`.

Проверка чарта выполнена из каталога `lab4`:

```bash
helm lint ./helm/shop
```

Helm проверил один чарт: `1 chart(s) linted, 0 chart(s) failed`.
Замечание `icon is recommended` носит рекомендательный характер.

---

# Часть 2 — Тесная площадка

### Кластер kind

Создан новый кластер `lab4` на Kubernetes `v1.37.0`: одна control-plane-нода
и одна worker-нода. Конфигурация хранится в `kind.yaml`.
На control-plane снят стандартный taint, чтобы обе ноды могли принимать
поды приложения. Это позволяет в дальнейшем проверить разнесение API,
не выходя за ограничение в две ноды.

Все команды выполняются из каталога `lab4`:

```bash
kind create cluster --name lab4 --config ./kind.yaml --wait 180s
kubectl --context kind-lab4 get nodes -o wide
kubectl --context kind-lab4 get nodes \
  -o custom-columns='NAME:.metadata.name,TAINTS:.spec.taints'
```

При первом запуске kind `0.30.0` из apt создание кластера завершилось ошибкой:
он передал kubeadm конфигурацию `v1beta3`, которую kubeadm `v1.37.0`
уже не принимает. После установки kind `v0.33.0` кластер успешно создан.

Обе ноды находятся в состоянии `Ready`, taints отсутствуют:

![Две готовые ноды kind без taints](images/part2/04-kind-nodes.png)

### CloudNativePG

Оператор CloudNativePG установлен отдельным Helm-релизом `cnpg`
в namespace `cnpg-system`. Для оператора заданы requests `100m CPU / 128Mi`
и limits `500m CPU / 256Mi`, а также метка `app: cnpg` для соответствия
политикам лабораторной 3.

Первый запуск был заблокирован загрузкой образа: containerd обращался
к прокси `127.0.0.1:10809`. В сетевом namespace kind-ноды этот адрес указывает
на саму ноду, где прокси отсутствовал, поэтому Pod получил `ImagePullBackOff`.
Прямая проверка registry GHCR из ноды вернула `HTTP 401`: registry
доступен и требует авторизации для этого запроса.

На обеих нодах через systemd drop-in
`/etc/systemd/system/containerd.service.d/zz-lab4-direct.conf` очищены
переменные прокси для containerd, затем сервис перезапущен.
После очередной попытки загрузки образа Pod оператора
`ghcr.io/cloudnative-pg/cloudnative-pg:1.30.1` перешёл в `1/1 Running`
без перезапусков. Повторный Helm upgrade до восстановления Pod успел
завершиться ошибкой `Progress deadline exceeded`; фактическую готовность
оператора подтвердили отдельной проверкой Pod.

После восстановления оператора повторный `helm upgrade --reuse-values`
завершился успешно: релиз `cnpg` перешёл в `deployed`, ревизия 3.
Используется чарт `cloudnative-pg-0.29.1`, версия оператора `1.30.1`.

![Успешное обновление Helm-релиза CloudNativePG](images/part2/05-cnpg-deployed.png)

### Kyverno

В namespace `kyverno` установлен чарт `kyverno-3.9.2`, версия Kyverno
`v1.19.2`:

```bash
helm upgrade --install kyverno kyverno/kyverno \
  --kube-context kind-lab4 \
  -n kyverno --create-namespace \
  --wait --timeout 5m
```

![Успешная установка Kyverno](images/part2/06-kyverno-deployed.png)

Для учебного кластера используется по одной реплике каждого контроллера.
Это экономит ресурсы тесной площадки, но не обеспечивает HA самого Kyverno.

### Проверка операторов

```bash
kubectl --context kind-lab4 get deployments -n cnpg-system
kubectl --context kind-lab4 get deployments -n kyverno
helm list --kube-context kind-lab4 -A
```

Deployment CloudNativePG и четыре Deployment Kyverno готовы `1/1`.
Оба Helm-релиза находятся в состоянии `deployed`.

![Готовые операторы и Helm-релизы](images/part2/07-operators-ready.png)

### Ограждения из лабораторной 3

В новый кластер применены исходные политики Kyverno:

```bash
kubectl --context kind-lab4 apply -f ../lab3/policies/
kubectl --context kind-lab4 wait \
  --for=condition=Ready clusterpolicy --all --timeout=120s
kubectl --context kind-lab4 get clusterpolicies
```

Все пять политик готовы (`READY: True`): `no-latest`, `no-privileged`,
`require-app-label`, `require-limits`, `require-non-root`.
Они проверяют явный тег образа, отсутствие privileged, метку `app`,
лимиты CPU/памяти и запуск без root. Для namespace приложения исключения
не добавлялись; batch должен выполнять те же требования.

Kyverno предупреждает об устаревании API `kyverno.io/v1 ClusterPolicy`,
но установленная версия поддерживает его и успешно приняла политики.

![Пять готовых политик Kyverno](images/part2/08-policies-ready.png)

### Мониторинг

В новом kind-кластере установлен стек из лабораторной 2 —
`kube-prometheus-stack` с Prometheus, Grafana, Alertmanager,
kube-state-metrics и node-exporter. Использован исходный файл настроек
лабораторной 2 и дополнительный `helm/monitoring-values.yaml`, который
убирает подключения к отсутствующим в этом кластере Loki и webhook.

```bash
helm upgrade --install monitoring prometheus-community/kube-prometheus-stack \
  --kube-context kind-lab4 \
  -n lab2 --create-namespace \
  -f ../lab2/helm/monitoring-values.yaml \
  -f ./helm/monitoring-values.yaml \
  --wait --timeout 10m
```

![Установка Helm-релиза monitoring](images/part2/09-monitoring-deployed.png)

Используется чарт `kube-prometheus-stack-92.12.0`, версия Prometheus Operator
`v0.94.1`. Релиз `monitoring` находится в состоянии `deployed`, все семь Pod
работают и все их контейнеры готовы, перезапусков нет.

![Готовые Pod мониторинга и статус Helm-релиза](images/part2/10-monitoring-ready.png)

Стек установлен до приложения: унаследованный чарт shop уже создаёт
`ServiceMonitor` и `PrometheusRule`, которым нужны CRD Prometheus Operator.
В части 3 Prometheus будет источником реального потребления ресурсов для krr.

### Диагностика хранилища PostgreSQL

При первом развёртывании shop batch запустился, но PostgreSQL остался
в ожидании PVC, а API и worker перезапускались с `connection refused`
при подключении к базе. В логах local-path-provisioner найден отказ Kyverno:
служебный `helper-pod-create-pvc-*` не соответствовал требованиям метки `app`,
лимитов и non-root. Из-за этого provisioner не мог создать каталог тома.

Для устранения причины подготовлены манифесты:

- `infra/local-path-config.yaml` добавляет helper метку `app: local-path-helper`,
  requests `10m / 16Mi` и limits `100m / 64Mi`;
- `policies/require-non-root.yaml` сохраняет правило лабораторной 3,
  добавляя узкое исключение только для `helper-pod-*` в namespace
  `local-path-storage`. Это системный компонент, которому нужны права
  на создание и удаление каталогов hostPath; для Pod shop исключений нет.
- `infra/local-path-provisioner.yaml` задаёт самому контроллеру хранилища
  non-root UID `65532`, requests `25m / 64Mi` и limits `200m / 128Mi`.
  Его обновление позволяет перезапустить контроллер с соблюдением политик
  и начать новую попытку выделения тома без ожидания прежнего backoff.

Изменённые ConfigMap helper и политика non-root успешно применены.

После применения `infra/local-path-provisioner.yaml` новый Pod контроллера
запустился `1/1 Running`. В логах зафиксированы создание каталога тома
и `ProvisioningSucceeded`. PVC `shop-postgres-1` перешёл в `Bound`,
выделен том `1Gi` с режимом `RWO` и StorageClass `standard`.
CloudNativePG завершил initdb: Pod `shop-postgres-1` готов `1/1`,
кластер PostgreSQL находится в состоянии `Cluster in healthy state`.

Готовность далее проверяется отдельно от Helm, без длительного `--wait`.

### Развёртывание shop и batch

В чарт `helm/shop` добавлен Deployment `batch`. Число реплик, образ,
число CPU-воркеров, размер буфера памяти и requests/limits задаются
в `values.yaml`. Для первой проверки используется одна реплика с одним
CPU-воркером и буфером 64 MiB, requests `100m / 96Mi`, limits `500m / 192Mi`.
Процесс работает от UID `65532`, без повышения привилегий,
со сброшенными capabilities и файловой системой контейнера только для чтения.

API и worker собраны из исходников лабораторной 3. Образы
`lab3-api:v3`, `lab3-worker:v2`, `lab4-batch:v1` загружены в ноды через
`kind load docker-image --name lab4`. Приложение развёрнуто через Helm:

```bash
helm upgrade --install shop ./helm/shop \
  --kube-context kind-lab4 -n lab4 --create-namespace
```

После исправления хранилища выполнен повторный `helm upgrade`:
релиз `shop` находится в состоянии `deployed`, ревизия 2.
Проверка фактического состояния подтвердила:

| Компонент | Готовность |
| --- | --- |
| API | `3/3` |
| worker | `2/2` |
| batch | `1/1` |
| PostgreSQL | `1/1`, `Cluster in healthy state` |
| PVC PostgreSQL | `Bound`, `1Gi`, `RWO` |

API и worker восстановились автоматически после очередной попытки запуска
в CrashLoopBackOff. По 12 перезапусков относятся к периоду недоступности
PostgreSQL во время первоначального развёртывания.

![Готовые Pod shop, Deployment, PostgreSQL и PVC](images/part2/11-shop-ready.png)

**Результат:** shop и batch работают в kind-кластере строго на двух нодах.
Batch развёрнут через чарт с requests/limits и соблюдением политик приложения. ✅

---

# Часть 3 — Реальные числа вместо угадывания

### Исходные ресурсы

До анализа krr используются следующие значения из `values.yaml`:

| Компонент | CPU request | CPU limit | Memory request | Memory limit |
| --- | --- | --- | --- | --- |
| API | `50m` | `200m` | `32Mi` | `128Mi` |
| worker | `50m` | `200m` | `32Mi` | `128Mi` |
| PostgreSQL | `100m` | `500m` | `256Mi` | `512Mi` |
| batch | `100m` | `500m` | `96Mi` | `192Mi` |

### Сценарий нагрузки

Подготовлен `load/orders.js` для k6: создание заказов через `POST /order`
с ожидаемым ответом `201`. Этот запрос выполняет INSERT в PostgreSQL,
поэтому нагружается критический путь `api → postgres`.
По умолчанию сценарий задаёт 20 запросов в секунду на протяжении 5 минут.
Фиксированная интенсивность позволяет заметить отставание сервиса:
если генератор не успевает начать запланированные итерации,
проверка `dropped_iterations` завершится ошибкой.

Перед нагрузкой проверен запрос `POST /order`: получен `HTTP 201 Created`
и создан заказ `lab4-baseline-check` с идентификатором `1`.

Генератор k6 запущен отдельным Docker-контейнером в сетевом namespace
ноды `lab4-worker`. Запросы идут на ClusterIP Service `shop-api`,
чтобы использовать балансировку между репликами API. Нагрузка генератора
не размещается отдельным Pod в тесном кластере.

### Исходный замер

Выполнен пятиминутный прогон k6 `2.3.0` с интенсивностью 20 запросов/с:

| Показатель | Результат |
| --- | --- |
| Создано заказов | `6000` |
| Ответы `201` | `100%` |
| Ошибки HTTP | `0%` |
| Пропущенные итерации | `0` |
| Средняя задержка HTTP | `6.32 ms` |
| p95 задержки HTTP | `9.04 ms` |
| Максимальная задержка HTTP | `514.63 ms` |

![Результат исходного нагрузочного прогона](images/part3/12-baseline-load.png)

Этот замер фиксирует исходное поведение приложения с одной репликой batch.
Подтверждение SLA при preemption и давлении по памяти выполняется в части 8.

### Получение рекомендаций krr

Использован официальный образ
`us-central1-docker.pkg.dev/genuine-flight-317411/devel/krr:v1.30.0`.
Штатный discovery krr не поддерживает CloudNativePG Cluster:
оператор управляет Pod напрямую, а не через StatefulSet.
Поэтому `krr/cnpg.py` добавляет обнаружение настоящих CNPG Cluster
через Kubernetes API, проверяя owner UID их работающих PostgreSQL-подов.
В отчёте база отображается как `Cluster/shop-postgres`, контейнер `postgres`.
Сбор метрик из Prometheus, стратегия `simple`, округление и минимальные
значения остаются штатными механизмами krr. Адаптер также сохраняет
структурированный JSON того же расчёта, который выводится таблицей.

```bash
bash ./krr/run.sh
```

Скрипт использует сетевой namespace kind-ноды для доступа к Prometheus,
временный внутренний kubeconfig удаляется после завершения.
В Kubernetes выполняются только операции чтения.
Результат сохранён в [krr/report.json](krr/report.json).

Для короткого лабораторного эксперимента заданы окно анализа 1 час,
шаг 1 минута и минимум 10 точек вместо стандартных 100.
Стратегия использует p95 CPU и максимум памяти с запасом 15%,
с учётом минимальных рекомендаций krr: 10m CPU и 100Mi памяти.
Предупреждение `Not enough history available` сохранено в отчёте:
набор данных короче рекомендуемых стратегией трёх часов.
Числовые рекомендации для всех четырёх компонентов получены;
они относятся к этому учебному профилю нагрузки.

![Обнаружение CNPG и расчёт рекомендаций четырёх компонентов](images/part3/13-krr-discovery.png)

![Параметры стратегии и таблица рекомендаций krr](images/part3/14-krr-recommendations.png)

| Компонент | CPU request до → krr | Memory request до → krr | Memory limit krr |
| --- | --- | --- | --- |
| API | `50m → 10m` | `32Mi → 100Mi` | `100Mi` |
| worker | `50m → 10m` | `32Mi → 100Mi` | `100Mi` |
| PostgreSQL | `100m → 16m` | `256Mi → 100Mi` | `100Mi` |
| batch | `100m → 501m` | `96Mi → 100Mi` | `100Mi` |

При 20 заказах/с сервисы критического пути расходуют меньше CPU,
чем предполагалось изначально. Batch, наоборот, непрерывно нагружает CPU
и достигает примерно 0,5 ядра — своего установленного лимита.
Значение рекомендации batch `501m` получилось после округления наблюдений,
оно не доказывает устойчивого превышения CPU-квоты.
Рост рекомендации памяти API и worker до 100Mi связан также с минимальным
порогом krr: это не означает фактическое потребление ими 100Mi.

### Перенос рекомендаций в values

В `helm/shop/values.yaml` перенесены CPU requests и requests/limits памяти
API, worker и PostgreSQL. Стратегия `simple` рекомендует убрать CPU limit
(`unset`), что несовместимо с ограждениями лабораторной. Поэтому на этом
этапе сохранены прежние конечные CPU limits: `200m` для API и worker,
`500m` для PostgreSQL. Эти CPU limits не являются рекомендациями krr.
Исходный JSON-отчёт сохранён без изменения рекомендаций.
В части 5 CPU limits критического пути будут приравнены к CPU requests,
полученным от krr, для выполнения требования Guaranteed.

| Компонент | CPU request | CPU limit | Memory request | Memory limit |
| --- | --- | --- | --- | --- |
| API | `10m` | `200m` | `100Mi` | `100Mi` |
| worker | `10m` | `200m` | `100Mi` | `100Mi` |
| PostgreSQL | `16m` | `500m` | `100Mi` | `100Mi` |

Ресурсы batch сохранены: они задают условия экспериментальной нагрузки,
а требование части 3 касается подбора ресурсов API, worker и PostgreSQL.
У всех контейнеров по-прежнему есть лимиты CPU и памяти.

Команда обновления из каталога `lab4`:

```bash
helm upgrade shop ./helm/shop --kube-context kind-lab4 -n lab4
```

Обновление создало ревизию 3 релиза `shop`. Изменение ресурсов PostgreSQL
потребовало перезапуска единственного экземпляра базы. CNPG сначала
запросил smart shutdown и ждал закрытия соединений до 180 секунд.
На время остановки базы новые API и worker завершались с
`connection refused`, затем восстановились при очередной попытке запуска.

Промежуточная проверка показывала API `READY 3/3` и worker `READY 2/2`,
но `UP-TO-DATE 1` у обоих Deployment: готовность обеспечивали старые Pod.
Кроме того, `/health` проверяет процесс API, а не доступность PostgreSQL,
поэтому такая готовность сама по себе не подтверждает работу пути заказа.

![Промежуточное состояние незавершённого обновления](images/part3/15-rollout-in-progress.png)

После завершения rollout отдельно проверены текущие ресурсы всех Pod:
у трёх API — `10m / 200m CPU`, `100Mi / 100Mi` памяти;
у двух worker — такие же значения;
у PostgreSQL — `16m / 500m CPU`, `100Mi / 100Mi` памяти
(в каждой паре requests / limits).
API имеет `READY 3/3`, `UP-TO-DATE 3`, worker — `READY 2/2`, `UP-TO-DATE 2`.
PostgreSQL вернулся в `Cluster in healthy state`.

![Завершённый rollout и применённые ресурсы krr](images/part3/16-krr-resources-applied.png)

**Результат:** рекомендации krr для API, worker и PostgreSQL перенесены
в values и применены через Helm; rollout завершён. ✅

---

# Часть 4 — Разнесение критического сервиса

В шаблон Deployment API добавлен выбор механизма размещения через
`scheduling.mechanism`. Ветка `topologySpreadConstraints` использует
следующие параметры `values.yaml`:

```yaml
scheduling:
  mechanism: topologySpreadConstraints
  topologyKey: kubernetes.io/hostname
  topologySpread:
    maxSkew: 1
    minDomains: 2
    whenUnsatisfiable: DoNotSchedule
    matchLabelKeys:
      - pod-template-hash
```

Топологический домен — отдельная нода. При двух доступных нодах разница
числа реплик не должна превышать одну: для трёх реплик ожидается `2+1`.
`DoNotSchedule` запрещает новые размещения, нарушающие ограничение.
`minDomains: 2` задаёт минимум двух доступных доменов для обычного расчёта
баланса: если их меньше, глобальный минимум принимается равным нулю.
Это не создаёт недостающую ноду и может оставлять новые Pod в Pending.

Селектор учитывает `app: api` и имя Helm-релиза, поэтому worker, batch
и поды другого релиза не участвуют в подсчёте. `pod-template-hash`
позволяет считать баланс каждой ревизии Deployment отдельно во время
обновления, когда одновременно существуют старые и новые Pod.

Альтернативная ветка `podAntiAffinity` использует жёсткое правило
`requiredDuringSchedulingIgnoredDuringExecution` на том же topologyKey.
При трёх репликах и двух нодах она оставит третью реплику Pending,
что объясняет выбор topology spread для этой площадки.
Неизвестное значение `scheduling.mechanism` останавливает рендеринг чарта.

Разнесение снижает риск одновременной потери всех реплик API при отказе
одной ноды. Оно не устраняет единую точку отказа PostgreSQL:
в этой лабораторной у базы один экземпляр и локальный том.
Доступность всего пути заказа нельзя вывести только из размещения API.

Применение правила выполняется через Helm:

```bash
helm upgrade shop ./helm/shop --kube-context kind-lab4 -n lab4
```

После обновления Deployment API готов `3/3`, `UP-TO-DATE 3`.
Две реплики размещены на `lab4-control-plane`, одна — на `lab4-worker`.
Все три Pod работают без перезапусков.

![Реплики API распределены между двумя нодами](images/part4/17-api-spread.png)

**Результат:** выбранный механизм применён через чарт, распределение `2+1`
подтверждено `kubectl get pods -o wide`. ✅

---

# Часть 5 — Приоритеты и гарантии

### QoS и приоритеты

В чарт добавлен шаблон `priority-class.yaml`. Значения приоритетов,
политика preemption и выбор класса для сервисов задаются в `values.yaml`.
Классы не являются глобальными по умолчанию.

| Компонент | PriorityClass | Приоритет | CPU requests = limits | Память requests = limits | Целевой QoS |
| --- | --- | --- | --- | --- | --- |
| API | `shop-critical` | `100000` | `10m` | `100Mi` | Guaranteed |
| PostgreSQL | `shop-critical` | `100000` | `16m` | `100Mi` | Guaranteed |

Числа requests взяты из krr части 3. Для Guaranteed CPU limits приравнены
к этим requests, а не удалены, как предлагает стратегия `simple`.
Равенство выполняется для CPU и памяти у всех контейнеров Pod.
Проверка существующего Pod CNPG показала, что `bootstrap-controller`
получает те же ресурсы, что и основной контейнер PostgreSQL.
Фактический QoS после обновления проверяется через `.status.qosClass`.

После Helm upgrade все три новые реплики API и PostgreSQL получили
`Guaranteed`, класс `shop-critical` и приоритет `100000`.
Batch получил `Burstable`, `shop-batch`, `-1000`.
Оба worker остались `Burstable` с приоритетом `0`.
Deployment API готов `3/3` и полностью обновлён; batch — `1/1`, worker — `2/2`.

![Фактические QoS и приоритеты после обновления](images/part5/18-qos-priorities.png)

Batch получает `shop-batch` с приоритетом `-1000` и остаётся Burstable:
requests `100m / 96Mi` заметно ниже limits `500m / 192Mi`.
Worker сохраняет приоритет по умолчанию `0` и Burstable.
Политика классов `PreemptLowerPriority` позволяет планировщику освобождать
место для нового высокоприоритетного Pod за счёт более низких приоритетов.
Сам по себе уже работающий высокий приоритет не запускает preemption.

Приоритет и QoS решают разные задачи. Scheduler выбирает жертв preemption
по приоритету и возможности разместить ожидающий Pod; QoS не является
критерием этого выбора. При давлении по памяти kubelet сначала учитывает,
превышает ли потребление requests, затем приоритет, затем величину
потребления относительно requests. Поэтому Burstable batch, превышающий
memory request и имеющий низкий приоритет, менее защищён, чем критический
Guaranteed Pod. Одного названия Burstable недостаточно, чтобы гарантировать
порядок жертв: это нужно проверить при фактическом давлении в части 7.
Guaranteed также не отменяет собственный memory limit и CPU throttling.

### PodDisruptionBudget

В `pod-disruption-budget.yaml` добавлены бюджеты:

| PDB | minAvailable | Смысл |
| --- | --- | --- |
| `shop-api` | `2` | При штатном обслуживании через Eviction API сохраняются минимум две доступные реплики API |
| `shop-postgres-critical` | `1` | Единственный primary нельзя добровольно выселить через Eviction API |

Автоматический PDB CloudNativePG отключён через
`postgres.operatorPDBEnabled: false` → `Cluster.spec.enablePDB: false`.
Вместо него чарт создаёт собственный PDB с селектором primary, чтобы один
Pod не попадал одновременно под два разных бюджета.

Первая проверка обнаружила только `shop-api`. Первоначальное имя PDB базы
`shop-postgres` совпало с именем бюджета реплик, которое резервирует CNPG.
Даже при `enablePDB: false` оператор выполняет очистку своих стандартных
имён. Поэтому имя PDB чарта изменено на `shop-postgres-critical`.

![Первичная проверка: PDB API есть, PDB PostgreSQL отсутствует](images/part5/19-pdb-missing-postgres.png)

PDB ограничивает добровольные disruptions через Eviction API. Он не
защищает от отказа ноды, OOM и node-pressure eviction, не запрещает прямое
удаление Pod контроллером при rollout. Для scheduler preemption соблюдение
PDB является best effort, а не абсолютным запретом.

### Штатная остановка PostgreSQL

Для локального эксперимента в values заданы `smartShutdownTimeout: 10`
и `stopDelay: 60` секунд. Они передаются в CNPG Cluster через чарт:
сначала PostgreSQL может дождаться закрытия соединений, затем оператор
использует штатный fast shutdown. Это сокращает ожидание открытых
соединений при последующих обновлениях вместо прежнего окна 180 секунд.
Сокращение окна относится к этой учебной базе.

```bash
helm upgrade shop ./helm/shop --kube-context kind-lab4 -n lab4
```

После исправления имени выполнен Helm upgrade до ревизии 6.
Проверка подтвердила два PDB: `shop-api` с `minAvailable: 2`
и одной разрешённой disruption, `shop-postgres-critical` с
`minAvailable: 1` и нулём разрешённых disruptions.

![Оба PDB созданы и действуют](images/part5/20-pdb-ready.png)

**Результат:** API и PostgreSQL имеют Guaranteed и высокий приоритет,
batch — Burstable и низкий приоритет; оба бюджета PDB подтверждены. ✅

---

# Часть 6 — Нехватка ресурсов и preemption

### Подготовка площадки

Перед экспериментом проверены реальные allocatable и CPU requests всех
размещённых Pod. На `lab4-control-plane` доступно `16000m` CPU,
requests подов вне batch суммарно составляют `1220m`.
Оставшиеся `14780m` позволяют разместить ровно два batch с request `7390m`.
Третья реплика должна остаться Pending по CPU. Batch закреплён за этой нодой
через `batch.nodeSelector` в values, чтобы свободная worker-нода не скрывала
нехватку на выбранной площадке. Здесь находится одна из трёх реплик API.

В values подготовлены `batch.replicas: 3`, CPU request `7390m`,
CPU limit `14780m`. Буфер остаётся 64 MiB, один CPU-воркер, память
requests `96Mi`, limits `192Mi`. Большой request намеренно резервирует CPU
для демонстрации планирования по requests; это не рекомендация krr для batch.
Сам процесс по-прежнему использует один CPU-воркер и не начинает потреблять
7,39 ядра из-за изменения request. Все лимиты и требования политик сохранены.

### Параллельная нагрузка на критический путь

Для наблюдения критического пути используется k6 с записью временного ряда в
`load/results/stress.json.gz`. Цель для последующей проверки части 8
фиксируется заранее: при 20 заказах/с p95 HTTP-задержки менее 300 мс,
доля HTTP-ошибок менее 1%, пропущенных итераций нет.
Нагрузка должна продолжаться во время экспериментов частей 6–7.

Первая попытка фонового запуска k6 завершилась до подачи нагрузки:
команда выполнялась из корня репозитория, поэтому в Docker был смонтирован
неверный каталог, а `/scripts/orders.js` не найден.
Этот запуск не дал измерений SLA; для фиксации preemption требуется
повторный запуск из `lab4`.

### Pending по requests

После Helm upgrade две реплики новой ревизии batch запустились
на `lab4-control-plane`, третья осталась Pending. Старая реплика
на `lab4-worker` сохранилась, поскольку RollingUpdate пока не может
завершить замену при неготовой третьей реплике.

![Две новые batch работают, третья ожидает размещения](images/part6/21-batch-pending.png)

У ожидающего Pod `shop-batch-7b5b56b65d-b5jx5`:
CPU request `7390m`, limit `14780m`, приоритет `-1000`, QoS Burstable,
`PodScheduled: False`. Событие `FailedScheduling` указывает
`1 Insufficient cpu` и одну ноду, не подходящую по selector.
Control-plane заполнен по requests, worker исключён настройкой размещения.
Scheduler не использует текущий фактический расход CPU для этой проверки.

![Ресурсы и состояние ожидающего batch](images/part6/22-pending-reason.png)

![FailedScheduling: нехватка CPU и ограничение nodeSelector](images/part6/23-pending-reason.png)

Для ожидающего batch preemption не нашёл жертв: остальные batch имеют
такой же приоритет, а критические и системные Pod — более высокий.

### Подготовка preemption

В values число реплик API увеличено с трёх до четырёх. В текущем размещении
одна реплика API находится на control-plane, две на worker. Ограничение
разнесения требует разместить четвёртую на control-plane для баланса `2+2`.
Свободных CPU requests там нет, поэтому scheduler должен освободить
место за счёт низкоприоритетного batch. CPU request API остаётся `10m`
из рекомендаций krr.

### Наблюдение preemption

После Helm upgrade до ревизии 8 scheduler вытеснил
`shop-batch-7b5b56b65d-25dbd` с `lab4-control-plane` для размещения
нового API с UID `0b0cbba4-5264-42bc-827a-78a6a601ff7d`.
Событие `Preempted` подтверждает выбор низкоприоритетного batch в жертвы.

![Batch вытеснен scheduler ради нового API](images/part6/24-preemption.png)

Новая реплика API получила место, однако не стала готовой из-за
недоступности PostgreSQL. В проверке API имел `3/4` готовых реплики,
PostgreSQL — `0/1`; часть batch оставалась Pending.

![Состояние после preemption: выявлена неготовность PostgreSQL](images/part6/25-after-preemption.png)

### Коррекция ресурсов PostgreSQL

У PostgreSQL выявлены таймауты startup/readiness/liveness probes.
Последний завершившийся контейнер имел `exitCode: 137`, `reason: Error`,
без отметки `OOMKilled`; в логах зафиксированы SIGTERM и штатная попытка
остановки после ошибок probes. Сам код 137 не является доказательством OOM.

Счётчик cgroup CPU показал `nr_periods: 2315`, `nr_throttled: 2267`:
ограничение CPU срабатывало примерно в 98% периодов.
Приравнивание CPU limit к p95-рекомендации requests `16m` оказалось
недостаточным для PostgreSQL и его instance-manager.
Guaranteed определяет класс обслуживания, но не исправляет слишком малую
CPU-квоту.

Неудачный прогон остановлен; метрики сохранены отдельно в
`load/results/stress-first-attempt.json.gz`. Получено 8631 HTTP-запрос,
из них 8416 ошибочных; p95 задержки около 5002 мс.
Этот прогон не подтверждает заданный SLA.

Для коррекции выполнен повторный расчёт встроенной стратегией krr
`simple-limit`: CPU request и limit рассчитываются по 100-му процентилю
наблюдений. Окно расширено до двух часов, чтобы включить исходные измерения
до установки CPU limit `16m`. Шаг остаётся одна минута, минимум — 10 точек.

```bash
KRR_STRATEGY=simple-limit KRR_REPORT=guaranteed-report.json KRR_HISTORY=2 \
  bash ./krr/run.sh --cpu-request 100 --cpu-limit 100
```

Новый расчёт сохранён в [krr/guaranteed-report.json](krr/guaranteed-report.json).
Для PostgreSQL получены CPU requests = limits `37m`,
память requests = limits `103Mi`. Эти значения перенесены в values
для проверки восстановления базы; Guaranteed и высокий приоритет сохранены.
До подтверждения готовности и нового нагрузочного прогона считать
коррекцию доказанно достаточной нельзя.

После Helm upgrade объект Cluster содержал новые ресурсы, однако старый Pod
оставался с `16m / 100Mi`: оператор не мог получить статус instance-manager
по HTTP и завершить обновление. Для восстановления удалён только зависший
Pod командой `kubectl delete pod shop-postgres-1 --wait=false`.
PVC сохранён; CNPG создал Pod заново с ресурсами из обновлённого чарта.

Повторная проверка показала `Cluster in healthy state`, одну готовую
инстанцию и `READY: true`. Фактические CPU request и limit равны `37m`,
лимит памяти — `103Mi`, QoS — Guaranteed. Это подтверждает восстановление
базы и применение настроек; соблюдение SLA требует отдельного прогона.

Минутная проверка при 20 заказах/с после восстановления не прошла:
1179 HTTP-запросов, 53 ошибки (4,49%), p95 — 2,49 с,
22 пропущенные итерации. Измерения сохранены в
`load/results/recovery-check.json.gz`. PostgreSQL повторно перезапустился;
в логах instance-manager зафиксированы таймауты и потеря lease.
Следовательно, `37m / 103Mi` не подтвердили достаточность под нагрузкой.

Для повторного измерения подготовлен временный файл
`helm/calibration-values.yaml`: исходные лимиты API `200m / 128Mi`
и PostgreSQL `500m / 512Mi` используются также как requests,
чтобы сохранить Guaranteed. Это профиль сбора измерений, а не итоговая
рекомендация. Расчёт по потреблению, уже ограниченному слишком малой
CPU-квотой, не показывает полный спрос приложения на CPU.

### Нагрузочная проверка с профилем измерений

Трёхминутный прогон при 20 заказах/с с временным профилем завершился
успешно: все 3601 запрос получили ожидаемый HTTP 201,
ошибок и пропущенных итераций нет. Средняя задержка — 6,93 мс,
p95 — 8,85 мс, максимум — 551,8 мс. Все пороги k6 выполнены:
единичный максимум выше 300 мс не нарушает условие для 95-го процентиля.
Временной ряд сохранён в `load/results/calibration.json.gz`.

![Успешное выполнение порогов k6 при 20 заказах в секунду](images/part6/26-calibration-thresholds.png)

![Задержки, число запросов и отсутствие пропущенных итераций](images/part6/27-calibration-results.png)

Этот результат подтверждает выполнение заданных порогов в данном прогоне
при текущей нагрузке batch. Он не заменяет проверку SLA во время повторного
preemption и давления по памяти. Следующий расчёт krr должен учитывать
измерения успешного прогона; профиль измерений не считается итоговой
рекомендацией ресурсов.

Повторный запуск krr с `history_duration=0.25` завершился на проверке
параметров: установленная версия требует окно истории не менее одного часа.
Рекомендации в этом запуске не получены. Для повторного расчёта окно
увеличивается до одного часа с сохранением шага 30 секунд.

![Отклонение недопустимого окна истории krr](images/part6/28-krr-history-validation.png)

### Повторный расчёт и результат части 6

При окне истории один час и шаге 30 секунд krr успешно рассчитал
рекомендации. Отчёт сохранён в
[krr/calibrated-report.json](krr/calibrated-report.json).
Для API получены requests = limits `10m / 100Mi`,
для PostgreSQL — `40m / 103Mi`. krr предупредил о недостаточной
длительности истории относительно требований стратегии, поэтому
результаты относятся к короткому лабораторному наблюдению.

![Рекомендации krr после успешного нагрузочного прогона](images/part6/29-krr-calibrated.png)

Эти рекомендации пока не применены: успешный прогон выполнен
с временным профилем `200m / 128Mi` для API и `500m / 512Mi`
для PostgreSQL. Максимум усреднённого потребления за окно измерения
не гарантирует достаточную CPU-квоту для коротких пиков; близкий
профиль PostgreSQL `37m / 103Mi` ранее не выдержал проверку.
Итоговое согласование ресурсов с рекомендациями и проверка SLA
остаются незавершёнными.

Наблюдения части 6 выполнены: batch остался Pending из-за нехватки
CPU requests и ограничения размещения, а scheduler вытеснил
низкоприоритетный batch для размещения API с высоким приоритетом.
Успешная нагрузочная проверка после восстановления базы зафиксирована
отдельно; она не доказывает соблюдение SLA непосредственно в момент
preemption. Повторная проверка этого момента относится к части 8.

# Часть 7 — Давление по памяти и eviction

## Подготовка

У kind-ноды отсутствует отдельный Docker memory limit, а конфигурация
kubelet не содержит порога `memory.available` в `evictionHard`.
Перед экспериментом требуется настроить контролируемый порог eviction,
чтобы не заполнять всю память ноутбука. Пересечение этого порога и
выбор жертвы kubelet необходимо подтвердить событиями; одного
увеличения потребления batch недостаточно для вывода об eviction.

Подготовлен `helm/memory-baseline-values.yaml`: одна реплика batch
размещается на `lab4-worker`, CPU request возвращается к `100m`,
limit — к `500m`. Память процесса пока составляет `64Mi`, request —
`96Mi`, limit — `1536Mi`. Batch сохраняет Burstable и низкий приоритет.
Профиль применяется вместе с работающим профилем критических сервисов
`helm/calibration-values.yaml`.

Попытка переключить batch на Recreate была отклонена API:
при server-side apply сохранился блок `rollingUpdate` прежней стратегии,
несовместимый с Recreate. Профиль не применился.
В исправленном профиле используется RollingUpdate с `maxSurge: 0`
и `maxUnavailable: 100%`: контроллер может остановить все старые
реплики, не ожидая готовности новой, и не создаёт дополнительные
реплики сверх заданного числа. Простой допустим для фонового batch.
Результаты давления по памяти и порядок жертв пока не получены.

Helm upgrade до ревизии 13 успешно применил подготовительный профиль.
Rollout batch завершился: единственный Pod `shop-batch-db56b45f9-7lqjz`
получил `1/1 Running` на `lab4-worker`, старые реплики удалены.

Для контролируемого эксперимента подготовлен
`infra/memory-eviction.py`. Он сохраняет исходную конфигурацию kubelet
внутри worker, измеряет `memory.available` через kubelet stats API
и задаёт hard eviction threshold на 512 MiB ниже текущего значения.
Таким образом, для пересечения порога не требуется исчерпать физическую
память ноутбука. Это специально настроенный порог лабораторного стенда,
а не свидетельство исчерпания всей памяти хоста.
Период снятия MemoryPressure установлен в две минуты для наблюдения
состояния после eviction. Перед настройкой проверяется наличие
не менее 3 GiB доступной памяти хоста.

```bash
python3 ./infra/memory-eviction.py
```

Исходная конфигурация восстанавливается командой
`python3 ./infra/memory-eviction.py --restore`.
Настройка порога пока не подтверждает eviction: далее требуется увеличить
фактическое потребление batch через values и записать события kubelet.

Скрипт настройки успешно выполнен: на хосте доступно 5453 MiB,
`memory.available` worker перед настройкой составляло 13557 MiB.
Установлен порог `13045Mi`, запас до него — 512 MiB.
После перезапуска kubelet worker имел `Ready=True`,
`MemoryPressure=False`; API, PostgreSQL и один batch были готовы.

![Порог eviction настроен, исходное состояние ноды и сервисов](images/part7/30-memory-baseline.png)

Следующий профиль `helm/memory-pressure-values.yaml` увеличивает
потребление памяти batch с 64 до 1024 MiB. Request остаётся `96Mi`,
limit — `1536Mi`: предполагается пересечение порога памяти ноды
при потреблении ниже собственного лимита контейнера. Само применение
профиля не доказывает eviction — необходимы причина `Evicted`
и сообщение kubelet о нехватке памяти. Параллельная нагрузка на заказы
записывается отдельно в `load/results/memory-pressure.json.gz`.

## Наблюдение eviction

После Helm upgrade до ревизии 14 batch с `MEMORY_MIB=1024`
запустился на worker. Примерно через 13 секунд kubelet вытеснил Pod
`shop-batch-6d7db56d5b-6xrc8`. В `kubectl describe pod` зафиксированы:

- статус Pod `Failed`, причина `Evicted`;
- сообщение `The node was low on resource: memory`;
- порог `13045Mi`, доступно `13225572Ki` (около 12915,6 MiB);
- потребление batch `1053972Ki` (около 1029,3 MiB)
  при request `96Mi` и limit `1536Mi`;
- QoS Burstable и приоритет `-1000`.

![Причина Evicted и потребление batch ниже лимита контейнера](images/part7/31-batch-evicted.png)

Событие `Warning Evicted` выпущено kubelet. Worker остался
`Ready=True` и получил `MemoryPressure=True`. Новая реплика batch
осталась Pending: worker имел непереносимый taint, а control-plane
не подходил по nodeSelector.

Все четыре API и PostgreSQL сохранили `1/1 Running`; у PostgreSQL
и трёх API счётчики перезапусков оставались нулевыми, у четвёртого API
сохранились четыре прежних перезапуска. В наблюдаемом составе shop
жертвой давления по памяти стал batch, критические Pod сохранились.

![Событие eviction, MemoryPressure и готовность API и PostgreSQL](images/part7/32-memory-pressure-services.png)

## Почему это eviction, а не OOMKilled

Причина Pod `Evicted` и сообщение kubelet прямо указывают на пересечение
порога доступной памяти ноды. Batch потреблял больше своего request,
имел низкий приоритет и был выбран для освобождения памяти.
При выборе жертв kubelet учитывает превышение memory request,
приоритет и относительное превышение request; QoS связан с этой
защитой через настройки ресурсов, но не задаёт безусловный порядок
для всех возможных Pod.

Потребление batch было ниже его собственного лимита `1536Mi`.
При OOMKilled из-за лимита контейнера память ограничивает его cgroup,
а в состоянии контейнера ожидается причина `OOMKilled`.
Здесь таблица Pod показывала `ContainerStatusUnknown`, а контейнер —
код 137, но причина всего Pod была `Evicted`. Эти отображения
не заменяют диагноз из причины Pod и событий kubelet.

Механизм eviction части 7 подтверждён. Итог параллельной нагрузки k6
и графики ещё необходимо проверить для части 8.

## Восстановление стенда

После фиксации события Helm upgrade до ревизии 15 вернул профиль
`memory-baseline-values.yaml` без профиля давления: потребление batch
снижено до 64 MiB. Команда `python3 ./infra/memory-eviction.py --restore`
восстановила исходную конфигурацию kubelet и перезапустила его.
Обе ноды имели статус Ready, новый batch `shop-batch-db56b45f9-872jd`
работал с готовностью `1/1`, API и PostgreSQL оставались готовы.
Завершённый Pod с причиной Evicted сохранился как свидетельство
эксперимента; его строка `ContainerStatusUnknown` не означает,
что новый batch неисправен. На момент проверки фоновый k6
ещё выполнялся, итоговые пороги пока не проверены.

# Часть 8 — SLA под нагрузкой

## Условия проверки

Заранее установленный SLA: при 20 заказах/с не менее 95% запросов
завершаются быстрее 300 мс, доля HTTP-ошибок менее 1%, пропущенных
итераций нет. Используется POST `/order` с проверкой ответа HTTP 201.

Нагрузка k6 выполнялась параллельно эксперименту eviction части 7.
Первые и последние HTTP-точки временного ряда относятся к
18:44:25–18:49:25 МСК 24 октября 2026 года. Профиль потребления batch
увеличен в Helm-ревизии 14 в 18:44:43. Событие kubelet `Evicted`
имеет `firstTimestamp=2026-10-24T15:44:57Z` (18:44:57 МСК),
то есть находится внутри нагрузочного прогона.

## Результаты при давлении по памяти

| Показатель | Результат | Условие |
| --- | --- | --- |
| Запросы / созданные заказы | 6000 / 6000 | HTTP 201 |
| HTTP-ошибки | 0% | < 1% |
| p95 HTTP-задержки | 108,86 мс | < 300 мс |
| Средняя HTTP-задержка | 20,47 мс | Информационно |
| Максимальная HTTP-задержка | 474,30 мс | SLA задан для p95 |
| Пропущенные итерации | 0 | 0 |

![Все пороги k6 выполнены во время эксперимента eviction](images/part8/33-memory-load-results.png)

Графики построены по исходным HTTP-точкам
`load/results/memory-pressure.json.gz` скриптом `load/plot-results.py`.
Времена отмеченных событий сохранены с указанием источников
в `load/memory-events.json`. Точки показывают отдельные запросы,
линия p95 рассчитана по неперекрывающимся 10-секундным окнам;
итоговый p95 рассчитан по всем запросам прогона.

![Задержки заказов и p95 с отметкой eviction batch](images/part8/34-memory-latency.png)

Максимальный p95 среди 10-секундных окон составил 232,81 мс:
порог 300 мс выдержан также в этих окнах. Единичные запросы
превышали 300 мс, что не противоречит условию для 95-го процентиля.

![Доля HTTP-ошибок во время нагрузки и eviction](images/part8/35-memory-errors.png)

Под давлением деградировал batch: он был вытеснен, а его замена
осталась Pending. API и PostgreSQL сохранили готовность, все заказы
в данном прогоне созданы успешно. Таким образом, выбранный SLA
подтверждён для наблюдавшегося эксперимента давления по памяти.

## Ограничения результата

Успешная проверка проведена с профилем критических сервисов
`helm/calibration-values.yaml`. Рекомендации krr `10m / 100Mi`
для API и `40m / 103Mi` для PostgreSQL пока не проверены как
итоговые ресурсы: близкий профиль PostgreSQL ранее не выдержал нагрузку.
Окончательное выполнение требования о ресурсах критического пути
из krr остаётся открытым.

Первый прогон во время preemption части 6 не выдержал SLA.
Поэтому для полного выполнения части 8 необходимо повторно
подтвердить SLA непосредственно во время preemption с работающим
профилем ресурсов. Успешный прогон eviction этого результата не заменяет.
