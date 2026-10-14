# Лаба 3 — Платформа для shop

<!--
Краткое введение: что делаем в этой лабе и зачем.
Пара предложений: ограждения политикой, Helm-чарт shop, Postgres через оператора,
проверка устойчивости при падении control plane, мониторинг поверх стека из Лабы 2.
-->

## Содержание

- [Часть 0 — Сервисы](#часть-0--сервисы)
- [Часть 1 — Ограждения на кластер](#часть-1--ограждения-на-кластер)
- [Часть 2 — Чарт api и worker](#часть-2--чарт-api-и-worker)
- [Часть 3 — Postgres через оператора](#часть-3--postgres-через-оператора)
- [Часть 4 — Падение control plane](#часть-4--падение-control-plane)
- [Часть 5 — Мониторинг](#часть-5--мониторинг)

---

## Часть 0 — Сервисы

Так, ну навайбкодили сервис, вот че он умеет: \

**Стек:** Go, `net/http`, драйвер `pgx/v5`.

**`api` — эндпоинты:**
- `GET /health` — `ok`, либо 5xx при `HEALTH_FAIL=true`
- `POST /order` — пишет заказ в `orders`
- `GET /orders` — возвращает последние 200 заказов

**`worker`:** раз в секунду забирает пачку `processed=false` (`FOR UPDATE SKIP LOCKED`), помечает обработанными.

**Переменные окружения:**
- `DATABASE_URL` — строка подключения к Postgres
- `HEALTH_FAIL` — только для `api`, ломает readiness (нужно в части 2)


---

## Часть 1 — Ограждения на кластер

### Выбор движка политик

Выбрали движок Kyverno, по описанию он легче для входа, плюс не надо изучать новый язык, хотя он умеет только кубер, но нам этого хватит для лабы, берем!

### Установка движка

Ставим через helm, тут все понятно

```bash
helm repo add kyverno https://kyverno.github.io/kyverno/
helm repo update
helm install kyverno kyverno/kyverno -n kyverno --create-namespace --version 3.9.1
```

### Правила


| № | Правило | Что требует / запрещает | Польза |
|---|---------|-------------------------|--------|
| 1 | check-limits | есть ли ограничения ресурсов(limits/requests) | чтобы под не съел всю ноду |
| 2 | check-images | образы только из указанных источников | защита от скачивания вредоносных образов |
| 3 | check-labels | должны быть теги app, team | для распределения подов по группам/командам |
| 4 | deny-privileged | запрет привилигированного режима запуска пода | базовая безопасность от дурачков |
| 5 | deny-host-access | запрет доступа к хосту | изоляция от хоста |

``` bash
kubectl get clusterpolicy

NAME              ADMISSION   BACKGROUND   READY   AGE   MESSAGE
shop-guardrails   true        true         True    65m   Ready
```

### Проверка: пять нарушающих манифестов

Подготовили плохие манифесты, которые должны быть отклонены, они в папке `policies/bad`

Пробуем их применить и получаем ошибку от kyverno

<img src="images/part1/1limits.png" width="600" alt="отсутсвие лимитов">
<img src="images/part1/2images.png" width="600" alt="недоверенный источник образов">
<img src="images/part1/3labels.png" width="600" alt="нет метки team">
<img src="images/part1/4priv.png" width="600" alt="запуск в привилегированном режиме">
<img src="images/part1/5host.png" width="600" alt="есть доступ к хосту">

---

## Часть 2 — Чарт api и worker

### Структура чарта

<!--
Перечисли файлы: Chart.yaml, values.yaml, templates/deployment-api.yaml,
templates/deployment-worker.yaml, templates/service-api.yaml, _helpers.tpl.
-->

```
charts/shop/
├── Chart.yaml
├── values.yaml
└── templates/
    ├── _helpers.tpl
    ├── deployment-api.yaml
    ├── deployment-worker.yaml
    ├── service-api.yaml
    └── ...
```

### Что заложено в шаблоны под правила из части 1

<!--
Явно свяжи: каждое правило из части 1 отражено в шаблонах. Иначе почему
установка проходит без отклонений.
Например: resources обязательны → values.schema.json или дефолт в values;
образ из доверенного реестра → value image.repository с префиксом;
метки → helpers-функция commonLabels.
-->

<!-- TODO: таблица или список -->

### Установка

```bash
# TODO: helm install shop ./charts/shop -n shop --create-namespace
```

<!-- TODO: скрин — kubectl get pods -n shop (api x3, worker x2) -->

### Reconciliation: ручное удаление пода

<!--
Удалить под api руками → вернулся (ReplicaSet заметил несоответствие
и создал новый). Приложи последовательность команд и вывод.
-->

```bash
# TODO: kubectl delete pod ..., kubectl get pods -w
```

<!-- TODO: скрин — под исчез и вернулся -->

### Reconciliation: изменение replicas через helm upgrade

<!--
Поменять replicas в values.yaml, helm upgrade → Deployment обновлён,
ReplicaSet догнал новое число подов.
-->

```bash
# TODO
```

<!-- TODO: скрин — kubectl get pods после апгрейда -->

### Rolling update без потерь

<!--
Обновить версию образа, одновременно долбить /health в цикле.
Показать, что ни один запрос не провалился и поды обновляются по одному
(maxSurge/maxUnavailable из Deployment strategy).
-->

```bash
# TODO: команда фонового цикла curl /health + helm upgrade
```

<!-- TODO: скрин — вывод цикла curl (0 ошибок) + kubectl rollout status -->

### Сломанный rollout и rollback

<!--
Выставить HEALTH_FAIL=true → новые поды не проходят readiness → rollout
зависает, старые исправные остаются. Показать, что /health продолжает
отвечать. Затем helm rollback → кластер вернулся.
-->

```bash
# TODO
```

<!-- TODO: скрин — rollout завис, старые поды живы, /health отвечает -->
<!-- TODO: скрин — helm rollback и успешный возврат -->

---

## Часть 3 — Postgres через оператора

### Выбор оператора

<!--
CloudNativePG или Zalando. Кратко: почему выбрал, что даёт оператор
(самостоятельно поднимает поды БД, следит за репликацией, бэкапами и т.д.)
-->

**Оператор:** <!-- CloudNativePG / Zalando -->

**Почему:** <!-- 2–4 предложения -->

### Установка оператора

```bash
# TODO: helm repo add ... && helm install ...
```

<!-- TODO: скрин — поды оператора в Running -->

### Объект БД в чарте

<!--
В чарте shop появился шаблон templates/cluster.yaml (или подобное) с CRD
оператора. Это значит: чарт сам декларирует "хочу Postgres", а оператор
материализует StatefulSet, Service, Secret и т.д.
-->

```yaml
# TODO: фрагмент шаблона cluster.yaml
```

### Reconciliation: ручное удаление пода postgres

<!--
Удалить под, оператор вернул. Сравни поведение с ReplicaSet: чем
оператор отличается от "просто контроллера Deployment".
-->

```bash
# TODO
```

<!-- TODO: скрин — под удалён и возвращён оператором -->

### `spec` и `status` объекта Cluster

<!--
kubectl get cluster -o yaml → приведи ключевые поля spec и status.
-->

**`spec`** содержит желаемое: <!-- версия Postgres, число инстансов, ресурсы, storage, бэкапы -->

**`status`** содержит наблюдаемое: <!-- текущие инстансы, primary, ready, условия -->

### Чем оператор отличается от controller-manager

<!--
Ключевые тезисы:
- controller-manager — встроенный компонент Kubernetes, крутит встроенные
  контроллеры (Deployment, ReplicaSet, StatefulSet и др.) — на "свои" типы.
- Оператор — паттерн: свой контроллер + CRD. Следит за пользовательским
  типом (Cluster от CloudNativePG) и умеет доменную логику (failover, бэкапы).
- Controller-manager generic, оператор — предметно-специфичен.
-->

<!-- TODO: сравнительный текст 5–8 предложений -->

<!-- TODO: скрин — kubectl get cluster -o yaml (фрагмент) -->

---

## Часть 4 — Падение control plane

### Как останавливал control plane

<!--
Опиши процедуру для kind (docker pause/stop на контейнере ноды, либо
остановка процесса etcd/apiserver внутри ноды). Для этого нужен доступ
к container runtime ноды (docker exec / crictl).
-->

```bash
# TODO
```

### Изменения не проходят

<!--
Попытка kubectl apply/helm upgrade → отказ, потому что apiserver недоступен
(или etcd не отвечает). Приложи вывод.
-->

```bash
# TODO
```

<!-- TODO: скрин — ошибка kubectl/helm при лежащем control plane -->

### Приложение продолжает отвечать

<!--
/health, /orders продолжают работать. Почему: kubelet на ноде продолжает
держать контейнеры (их некому останавливать), endpoints уже закэшированы
в kube-proxy, etcd не нужен для обслуживания уже созданных подов.
-->

```bash
# TODO: curl /health и /orders
```

<!-- TODO: скрин — успешный ответ от api при лежащем CP -->

### Восстановление

<!--
Вернуть control plane → kubectl снова отвечает, cluster снова управляем.
-->

```bash
# TODO
```

<!-- TODO: скрин — kubectl get nodes после восстановления -->

---

## Часть 5 — Мониторинг

<!--
Используем стек из Лабы 2 — ничего нового не поднимаем.
В чарт добавляем ServiceMonitor (скрейп /metrics) и PrometheusRule (3 алерта).
-->

### ServiceMonitor

<!--
api отдаёт /metrics? Если нет — добавь (в части 0 код не включает метрики,
но их можно взять из Лабы 2 или дописать). ServiceMonitor ссылается на
Service shop с label app=api.
-->

```yaml
# TODO: фрагмент templates/servicemonitor.yaml
```

### Три алерта

<!--
Выбрать 3 правила. Каждое обосновать: что ловит, чем грозит пользователю,
что делать дежурному. Примеры: api недоступен, доля 5xx у api, worker
не обрабатывает заказы дольше N минут.
-->

**Алерт 1: `<name>`**
- Метрика / выражение: <!-- PromQL -->
- Что ловит: <!-- одна фраза -->
- Чем грозит: <!-- боль пользователя -->
- Что делать: <!-- действия дежурного -->

**Алерт 2: `<name>`**
- ...

**Алерт 3: `<name>`**
- ...

```yaml
# TODO: фрагмент templates/prometheusrule.yaml
```

### Провокация firing

<!--
Спровоцировать хотя бы одно правило, показать firing в Prometheus UI
и (опционально) уведомление в Telegram/Alertmanager.
-->

```bash
# TODO: как провоцировал
```

<!-- TODO: скрин — Prometheus Rules: алерт в Firing -->
<!-- TODO: скрин — Alertmanager / Karma: алерт виден -->
<!-- TODO: скрин (опционально) — Telegram уведомление -->

---

## Итог

<!--
Короткое резюме: всё, что требовалось по заданию, работает.
Сквозная проверка: чарт ставится с нуля, поды возвращаются сами, control
plane падает — приложение живо, алерты firing.
-->

<!-- TODO: сквозной чек-лист по "Результат" из lab.md -->

---

## Что было неочевидным / на чём застревал

<!--
Полезно для отчёта: список граблей. Например:
- как отключить поведение Kyverno в specific namespace
- как настроить readiness probe так, чтобы rollout действительно завис
- как провернуть безопасно остановку etcd в kind
- почему Helm upgrade при лежащем CP показывает странную ошибку
-->

<!-- TODO: 3–5 пунктов -->