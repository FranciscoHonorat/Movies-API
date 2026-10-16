{{- define "movies-api.fullname" -}}
{{- printf "%s" .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "movies-api.labels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{/* Uso: include "movies-api.selector" (list $ "api-gateway") */}}
{{- define "movies-api.selector" -}}
{{- $root := index . 0 -}}
app.kubernetes.io/instance: {{ $root.Release.Name }}
app.kubernetes.io/component: {{ index . 1 }}
{{- end -}}

{{/* Nome de um componente: <release>-<componente> */}}
{{- define "movies-api.name" -}}
{{- $root := index . 0 -}}
{{- printf "%s-%s" (include "movies-api.fullname" $root) (index . 1) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "movies-api.image" -}}
{{- $root := index . 0 -}}
{{- $name := index . 1 -}}
{{- if $root.Values.image.registry -}}
{{ printf "%s/%s:%s" $root.Values.image.registry $name $root.Values.image.tag }}
{{- else -}}
{{ printf "%s:%s" $name $root.Values.image.tag }}
{{- end -}}
{{- end -}}
