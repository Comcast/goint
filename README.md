# Integration testing in Go
`goint` is an integration testing framework written in Go. It was written for use in applications developed in Go. While the implementation of an integration test using the framework must be written in Go, much of the functionality can be used for applications written in other languages as well. Some of the functionality could be used in contexts other than integration testing.

# Terms
<table border="1">
<thead>
<tr class="header">
<th align="left"><p>Term</p></th>
<th align="left"><p>Description</p></th>
</tr>
</thead>
<tbody>
</tr>
<tr>
<td align="left"><p>Some term here...</p></td>
<td align="left"><p>Some description here....
</p></td>
</tbody>
</table>

# Features
`goint` implements several capabilities which can be called verbs. `goint` supports the following verbs:
1. `Exec` - executes an arbitrary process. This can be anything that can be launched from a shell command line. One field in the `Exec` struct is `Token` which can be referenced in the `Kill` and `Wait` verbs.
1. `Kill` - used to kill an `Exec`'d process. The process to be killed is identified by the `Token` field value in the corresponding `Exec` definition.
1. `Delay` - does just that, it delays for a given amount of time and then continues
1. `Wait` - waits for a process `Exec`'d with the matching `Token` value to exit
1. `Curl` - will run an aribtrary curl request
1. `DelayHealthCheck` - pings an arbitrary URL for a given number of times waiting a given number of sections between polls. It will complete with either an error or success.
1. `GoFunc` - will run an arbitrary Go function.
1. `Comp` - combines an arbitrary set of the above verbs into a composite action. `Comp` verbs can be nested to create arbitrarily complex hierarchies of arbitrarily complex steps.

# Examples
See `goint_test.go`.
