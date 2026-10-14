{{/*
Common labels для всех объектов чарта.
*/}}
{{- define "shop.labels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Селекторные метки для пода api.
*/}}
{{- define "shop.api.selectorLabels" -}}
app: api
{{- end -}}

{{/*
Селекторные метки для пода worker.
*/}}
{{- define "shop.worker.selectorLabels" -}}
app: worker
{{- end -}}