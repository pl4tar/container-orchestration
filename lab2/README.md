# Лаба 2

Мониторинг сервиса `api` в Kubernetes: метрики, логи, трейсы, алерты. Всё развёрнуто через Helm в локальном кластере kind. По сути у нас есть сервис и два независимых канала наблюдающие за ним, которые складывают статистику в Grafana.

## Часть 0 — сервис и кластер

Навайбкодили простой сервис на Go, и на скрине дернули все ручки. 

<img src="images/part-0/server-metrics.png" width="600" alt="метрики сервиса на /metrics">

По пути `/metrics` собраны метрики для дашборда RED. 


Потом мы ставили **kind** вместо **minikube** (словили спойлер от одногруппника, что нельзя будет с миникубом уронить сервис в 3 лабе). Также мы разместили 3 ноды, по требованиям из задания. 

<img src="images/part-1/cluster-up.png" width="700" alt="ноды кластера в Ready">

<!-- TODO: подпись — ниже вариант Данила: кластер из одной ноды, сервис выкачен через kubectl apply -->

<img src="images/other/photo_2026-10-05%2015.32.28%20(1).jpeg" width="700" alt="создание кластера kind у Данила">

<img src="images/other/photo_2026-10-05%2015.32.29.jpeg" width="800" alt="нода кластера и загрузка образа через kind load у Данила">

<img src="images/other/photo_2026-10-05%2015.32.30.jpeg" width="800" alt="деплой сервиса через kubectl apply у Данила">

<img src="images/other/photo_2026-10-05%2015.32.28.jpeg" width="600" alt="под и сервис api у Данила">

## Часть 1 — метрики (Prometheus + Grafana)
<!-- TODO: что такое kube-prometheus-stack и что в нём пришло одним релизом -->

<!-- TODO: как Prometheus узнаёт о сервисе: ServiceMonitor → оператор → конфиг → scrape по IP пода.
     Какой селектор ServiceMonitor выбрал и почему -->

<!-- TODO: подпись — источники данных в Grafana, Prometheus подключён чартом -->

<img src="images/other/photo_2026-10-05%2015.32.23.jpeg" width="800" alt="источники данных в Grafana">

<!-- TODO: подпись — цель api в состоянии UP, скрейп идёт на IP пода -->

<img src="images/other/photo_2026-10-05%2015.32.26.jpeg" width="800" alt="цель api в состоянии UP в Prometheus">

<!-- TODO: подпись — ряды http_requests_total: лейблы namespace, pod, job, instance дописал Prometheus -->

<img src="images/other/photo_2026-10-05%2015.32.25.jpeg" width="800" alt="запрос http_requests_total в Prometheus">

### Дашборд RED

| Панель | Запрос | Что показывает |
|---|---|---|
| Rate | <!-- TODO --> | |
| Errors | <!-- TODO --> | |
| Duration | <!-- TODO --> | |

<!-- TODO: как дашборд доставляется в Grafana (ConfigMap + sidecar) и почему не через интерфейс -->

<!-- TODO: что делал для нагрузки (/load, /fail, /slow) и как отреагировали графики -->
Ниже предсталвены варианты графаны от нас двоих, но тут признаю, Данил выбрал более удачное отоброжение метрик, правда ошибки подкачали, я бы свой вариант оставил. 

<img src="images/part-2/grafana.png" width="800" alt="дашборд RED в Grafana от Артёма">
<img src="images/part-2/danil-Grafana.jpg" width="800" alt="дашборд RED в Grafana от Данила">

## Часть 2 — логи (Loki + Grafana)
---
<!-- TODO: разделение ролей: Loki хранит, Alloy собирает. Путь строки от stdout до Grafana -->

<img src="images/part-3/loki-install.png" width="700" alt="установка Loki">

<img src="images/other/photo_2026-10-05%2015.32.30%20(1).jpeg" width="700" alt="установка Loki у Данила">

<img src="images/part-3/loki-running.png" width="600" alt="поды Loki и Alloy">

<!-- TODO: что делает конфиг Alloy по шагам, какие лейблы назначаются и почему trace_id не лейбл -->

<!-- TODO: запрос LogQL, которым нашёл ошибку от /fail -->

<img src="images/part-3/logs.png" width="800" alt="ошибка /fail в логах Grafana">

<img src="images/other/photo_2026-10-05%2015.32.31.jpeg" width="800" alt="ошибка /fail в логах Grafana у Данила">

<!-- TODO: подпись — метрики и логи в одном окне: панель логов на дашборде RED -->

<img src="images/other/photo_2026-10-05%2015.32.32.jpeg" width="800" alt="дашборд с панелью логов из Loki">

## Часть 3 — трейсы (OpenTelemetry + Jaeger)
---
<!-- TODO: как сервис инструментирован, куда и по какому протоколу уходят спаны -->

<!-- TODO: подпись — список трейсов сервиса api: /slow с двумя спанами, /fail с ошибкой -->

<img src="images/other/photo_2026-10-05%2015.32.34%20(1).jpeg" width="800" alt="список трейсов сервиса api в Jaeger">

<!-- TODO: подпись — водопад /slow: время ушло во вложенный спан slow-op -->

<img src="images/other/photo_2026-10-05%2015.32.34.jpeg" width="800" alt="водопад трейса /slow со спаном slow-op">

<!-- TODO: подпись — трейс /fail: спан помечен ошибкой -->

<img src="images/other/photo_2026-10-05%2015.32.35.jpeg" width="800" alt="трейс /fail с ошибочным спаном">

<!-- TODO: подпись — строка лога /fail с trace_id df5a06223ae6c3aa1aa9ec4cb680f910; по этому же id открыт трейс на скриншоте выше -->

<img src="images/other/photo_2026-10-05%2015.32.34%20(2).jpeg" width="800" alt="строка лога /fail с trace_id в Grafana">

## Часть 4 — алерты (Alertmanager + Karma)
---
<!-- TODO: как правило из PromQL доходит до получателя: Prometheus → Alertmanager → получатель -->

### Три критичных алерта

| Алерт | Что ловит | Почему это важно | Что делать дежурному |
|---|---|---|---|
| `ApiDown` | <!-- TODO --> | | |
| `ApiHighErrorRate` | <!-- TODO --> | | |
| `ApiHighLatency` | <!-- TODO --> | | |

<!-- TODO: подпись — три правила загружены в Prometheus -->

<img src="images/other/photo_2026-10-05%2015.32.36.jpeg" width="700" alt="правила алертов api в Prometheus">

<!-- TODO: как спровоцировал каждый алерт -->

<!-- TODO: скриншот алертов в firing в Alertmanager -->

<!-- TODO: подпись — получатель: уведомления в Telegram -->

<img src="images/other/photo_2026-10-05%2015.32.38.jpeg" width="400" alt="уведомление ApiDown в Telegram">

<img src="images/other/photo_2026-10-05%2015.32.37.jpeg" width="400" alt="уведомление ApiHighErrorRate в Telegram">

<!-- TODO: подпись — те же алерты в Karma -->

<img src="images/other/photo_2026-10-05%2015.32.39.jpeg" width="800" alt="сработавшие алерты в Karma">

## Итог
---
<!-- TODO: сквозная проверка: жму /load, /fail, /slow → что вижу на дашборде, в логах, в Jaeger, в алертах -->

<!-- TODO: что было неочевидным и на чём застревал -->
