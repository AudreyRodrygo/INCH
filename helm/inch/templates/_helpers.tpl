{{/*
Expand the name of the chart.
*/}}
{{- define "inch.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "inch.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "inch.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Selector labels for a component
Usage: include "inch.selectorLabels" (dict "name" "collector" "root" .)
*/}}
{{- define "inch.selectorLabels" -}}
app.kubernetes.io/name: {{ include "inch.name" .root }}
app.kubernetes.io/component: {{ .name }}
{{- end }}

{{/*
Image reference helper
Usage: include "inch.image" (dict "cfg" .Values.collector "root" .)
*/}}
{{- define "inch.image" -}}
{{ .root.Values.image.registry }}/{{ .cfg.image.name }}:{{ .cfg.image.tag }}
{{- end }}
