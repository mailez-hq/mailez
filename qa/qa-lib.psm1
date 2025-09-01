# qa-lib.psm1 — shared helpers for mailez functional QA (PS 5.1)
# Base URL: override with $env:QA_BASE (e.g. the compose stack's
# http://127.0.0.1:8081/api/v1); defaults to the dev-run 8080.
$script:Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
$script:Results = New-Object System.Collections.Generic.List[string]

function New-Session {
    param([string]$Email = "admin@example.com", [string]$Pw = "MailezDemo2026!")
    $body = @{ email = $Email; pw = $Pw } | ConvertTo-Json -Compress
    $bytes = [Text.Encoding]::UTF8.GetBytes($body)
    $r = Invoke-WebRequest -Uri "$Base/sso/login" -Method POST -Body $bytes -ContentType "application/json" -UseBasicParsing -TimeoutSec 40 -SessionVariable s
    return @{ S = $s; Email = $Email; Status = [int]$r.StatusCode }
}

function Api {
    param($Sess, [string]$Method, [string]$Path, $Body = $null, [int]$Expect = 200)
    $params = @{ Uri = "$Base$Path"; Method = $Method; WebSession = $Sess.S; UseBasicParsing = $true; TimeoutSec = 60 }
    if ($null -ne $Body) {
        $json = if ($Body -is [string]) { $Body } else { $Body | ConvertTo-Json -Depth 8 -Compress }
        $params.Body = [Text.Encoding]::UTF8.GetBytes($json)
        $params.ContentType = "application/json"
    }
    try {
        $r = Invoke-WebRequest @params
        $json = $null
        try { $json = [Text.Encoding]::UTF8.GetString($r.RawContentStream.ToArray()) | ConvertFrom-Json } catch {}
        return @{ Status = [int]$r.StatusCode; Data = $json; Raw = $r }
    } catch {
        $code = 0
        if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
        return @{ Status = $code; Data = $null; Raw = $null; Err = $_.Exception.Message }
    }
}

function Check {
    param([string]$Id, [string]$Name, [bool]$Ok, [string]$Detail = "")
    $tag = if ($Ok) { "PASS" } else { "FAIL" }
    $line = "$tag $Id $Name" + $(if ($Detail) { "  -- $Detail" } else { "" })
    $script:Results.Add($line)
    Write-Output $line
}

function Flush-Results {
    param([string]$Section)
    $dir = "D:\code\mailez-hq\mailez\qa"
    $file = Join-Path $dir "results.log"
    $script:Results | Out-File -FilePath $file -Append -Encoding UTF8
    $script:Results.Clear()
}

function New-QaUser {
    param([string]$Email)
    $admin = New-Session
    $r = Api $admin POST "/users" @{ email = $Email; password = "QaUser2026!"; display_name = "QA $Email" }
    return @{ Email = $Email; Pw = "QaUser2026!"; Created = ($r.Status -eq 200 -or $r.Status -eq 409) ; CreateStatus = $r.Status }
}
Export-ModuleMember -Function *
