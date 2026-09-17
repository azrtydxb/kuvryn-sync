{{- define "solder.name" -}}solder{{- end -}}
{{- define "solder.fullname" -}}{{ .Release.Name }}-{{ include "solder.name" . }}{{- end -}}
