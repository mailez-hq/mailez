# qa-sse-listen.ps1 — open one authenticated SSE stream and append every frame
# to a log file with a millisecond timestamp, so a parent script can measure
# push latency and count keepalive pings. Runs as a separate process because
# the reader has to stay blocked while the parent drives other API calls.
param(
    [Parameter(Mandatory = $true)][string]$Url,
    [string]$Cookie = "",
    [string]$CookieFile = "",
    [Parameter(Mandatory = $true)][string]$Out,
    [int]$Seconds = 600
)
[Console]::OutputEncoding = [Text.Encoding]::UTF8

# The cookie is normally handed over in a file: the parent's ArgumentList joins
# arguments with spaces, which would split a Cookie header in transit.
if (-not $Cookie -and $CookieFile) {
    $Cookie = (Get-Content -LiteralPath $CookieFile -Raw -Encoding UTF8).Trim()
}
if (-not $Cookie) { throw "no cookie supplied" }

function Write-Line([string]$line) {
    Add-Content -LiteralPath $Out -Value ("{0} {1}" -f (Get-Date -Format 'yyyy-MM-ddTHH:mm:ss.fff'), $line) -Encoding UTF8
}

# Start the stream before the log exists so the parent can tell "not connected
# yet" from "connected, event pending".
Set-Content -LiteralPath $Out -Value "#start $(Get-Date -Format 'yyyy-MM-ddTHH:mm:ss.fff')" -Encoding UTF8

$handler = New-Object System.Net.Http.HttpClientHandler
$handler.UseCookies = $false
$handler.UseProxy = $false
$cli = New-Object System.Net.Http.HttpClient($handler)
$cli.Timeout = [TimeSpan]::FromSeconds($Seconds + 60)

$req = New-Object System.Net.Http.HttpRequestMessage([System.Net.Http.HttpMethod]::Get, $Url)
[void]$req.Headers.TryAddWithoutValidation("Cookie", $Cookie)
[void]$req.Headers.TryAddWithoutValidation("Accept", "text/event-stream")
[void]$req.Headers.TryAddWithoutValidation("Cache-Control", "no-cache")

try {
    $resp = $cli.SendAsync($req, [System.Net.Http.HttpCompletionOption]::ResponseHeadersRead).GetAwaiter().GetResult()
    Write-Line "#status=$([int]$resp.StatusCode) ctype=$($resp.Content.Headers.ContentType)"
    if ([int]$resp.StatusCode -ne 200) { Write-Line "#exit"; exit 1 }
    $reader = New-Object System.IO.StreamReader($resp.Content.ReadAsStream())
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($sw.Elapsed.TotalSeconds -lt $Seconds) {
        $line = $reader.ReadLine()
        if ($null -eq $line) { Write-Line "#eof"; break }
        Write-Line $line
    }
} catch {
    Write-Line "#error $($_.Exception.Message)"
}
Write-Line "#exit"
