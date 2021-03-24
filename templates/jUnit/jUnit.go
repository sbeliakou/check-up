package jUnit

const JUnitTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites time="">
	<testsuite name="{{ .SuitName }}" tests="{{ .TotalTests }}" failures="{{ .FailedTests }}" errors="0" skipped="0" time="{{ .TotalTime }}" timestamp="{{ .TimeStamp }}" hostname="">

	{{- $verbosity := .Verbosity }}
	{{- range $t := .Tests }}
		{{- if $t.CanShow }}
			{{- if $t.IsSuccessful }}
				<testcase classname="{{ $.SuitName }}" name={{ Quote $t.Case }} time="{{ $t.Duration }}">
					<!-- system-out>STDOUT text</system-out -->
				</testcase>
			{{- else }}
				<testcase classname="{{ $.SuitName }}" name={{ $t.Case | Quote }} time="{{ $t.Duration }}">
					<failure type="failure">{{ Quote $t.Stdout }}</failure>
				</testcase>
			{{-  end }}
		{{-  end }}
  {{- end }}
  </testsuite>
</testsuites>
`