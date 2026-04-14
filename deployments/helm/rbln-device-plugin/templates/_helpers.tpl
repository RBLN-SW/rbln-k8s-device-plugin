{{- define "rbln-device-plugin.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "rbln-device-plugin.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- include "rbln-device-plugin.name" . -}}
{{- end -}}
{{- end -}}

{{- define "rbln-device-plugin.namespace" -}}
{{- if .Values.namespaceOverride -}}
{{- .Values.namespaceOverride -}}
{{- else -}}
{{- .Release.Namespace -}}
{{- end -}}
{{- end -}}

{{- define "rbln-device-plugin.labels" -}}
app.kubernetes.io/name: {{ include "rbln-device-plugin.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "rbln-device-plugin.selectorLabels" -}}
app.kubernetes.io/name: {{ include "rbln-device-plugin.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- with .Values.selectorLabelsOverride }}
{{- toYaml . }}
{{- end }}
{{- end -}}

{{- define "rbln-device-plugin.fullimage" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}

{{- define "rbln-device-plugin.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "rbln-device-plugin.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}
