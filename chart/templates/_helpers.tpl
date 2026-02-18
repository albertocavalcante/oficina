{{- define "oficina.name" -}}
{{- .Chart.Name -}}
{{- end -}}

{{- define "oficina.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "oficina.labels" -}}
app.kubernetes.io/name: {{ include "oficina.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "oficina.serverLabels" -}}
{{ include "oficina.labels" . }}
app.kubernetes.io/component: server
{{- end -}}

{{- define "oficina.agentLabels" -}}
{{ include "oficina.labels" . }}
app.kubernetes.io/component: agent
{{- end -}}

{{- define "oficina.serverSelectorLabels" -}}
app.kubernetes.io/name: {{ include "oficina.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: server
{{- end -}}

{{- define "oficina.agentSelectorLabels" -}}
app.kubernetes.io/name: {{ include "oficina.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: agent
{{- end -}}
