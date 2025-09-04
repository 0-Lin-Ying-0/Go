# Changelog

{{- range .Versions }}
{{- range .CommitGroups }}

### {{ .Title }}

    {{- range .Commits }}

- {{ if .Scope }}**{{ .Scope }}:** {{ end }}{{.Hash.Short}}: {{ .Subject }} (@{{.Author.Name}})
  {{- end }}
  {{- end }}

  {{- if .RevertCommits }}

### Reverts

    {{- range .RevertCommits }}

- {{ .Revert.Header }}
  {{- end }}
  {{- end }}

  {{- if .MergeCommits }}

### Pull Requests

    {{- range .MergeCommits }}

- {{ .Header }}
  {{- end }}
  {{- end }}

  {{- if .NoteGroups }}
  {{- range .NoteGroups }}

### {{ .Title }}

      {{- if .Notes }}
        {{- range .Notes }}

{{ .Body }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}
