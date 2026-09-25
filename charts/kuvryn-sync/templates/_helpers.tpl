{{- define "kuvryn-sync.name" -}}kuvryn-sync{{- end -}}
{{- define "kuvryn-sync.fullname" -}}{{ .Release.Name }}-{{ include "kuvryn-sync.name" . }}{{- end -}}
