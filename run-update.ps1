#Requires -Version 5.1
<#
.SYNOPSIS
    Manually trigger the GitHub Actions subscription deployment workflow and wait for the result.

.DESCRIPTION
    Wraps the equivalent gh CLI sequence used for a manual update:
      1. gh workflow run <workflow> --repo <repo> --ref <ref>   (trigger)
      2. gh run watch <run-id> --exit-status                    (wait, fail loudly if the run fails)
      3. gh run view <run-id>                                   (print the run summary)

    Triggering the deploy.yml workflow regenerates clash.yaml from config.yaml
    on GitHub's runner and deploys it to GitHub Pages. Defaults target this
    repository (deploy.yml @ main). Requires the gh CLI to be installed and
    authenticated.

.EXAMPLE
    .\run-update.ps1

.EXAMPLE
    .\run-update.ps1 -NoWait          # trigger only, print the run URL and exit

.EXAMPLE
    .\run-update.ps1 -MihomoVersion v1.19.1     # pin the Mihomo release tag

.PARAMETER Repo
    GitHub repository in "owner/name" form. Default: muddyneil/pangolin.

.PARAMETER Workflow
    Workflow file name (or ID) to trigger. Default: deploy.yml.

.PARAMETER Ref
    Branch/ref the workflow runs against. Default: main.

.PARAMETER MihomoVersion
    Value for the workflow's mihomo_version input (Mihomo release tag). Default: latest.

.PARAMETER NoWait
    Trigger the run and exit immediately instead of watching it.

.PARAMETER SkipView
    Skip the gh run view summary after the run finishes.

.PARAMETER Interval
    Poll interval in seconds for gh run watch. Default: 15.
#>
[CmdletBinding()]
param(
    [string]$Repo = "muddyneil/pangolin",
    [string]$Workflow = "deploy.yml",
    [string]$Ref = "main",
    [string]$MihomoVersion = "latest",
    [switch]$NoWait,
    [switch]$SkipView,
    [int]$Interval = 15
)

# Intentionally NOT setting $ErrorActionPreference = "Stop": with it, native stderr
# captured via 2>&1 becomes a terminating error under Windows PowerShell 5.1,
# which would abort before our explicit $LASTEXITCODE handling below.

function Assert-GhAvailable {
    if (-not (Get-Command gh -ErrorAction SilentlyContinue)) {
        throw "gh CLI not found. Install it first: https://cli.github.com/"
    }
}

# Extract the run id from gh workflow run output (the run URL), with a fallback
# that polls for the matching workflow_dispatch run if the URL is not printed.
function Get-TriggeredRunId {
    param(
        [string]$TriggerOutput,
        [string]$Repo,
        [string]$Workflow,
        [datetime]$TriggeredAt,
        [string]$Ref
    )

    $match = [regex]::Match($TriggerOutput, 'actions/runs/(\d+)')
    if ($match.Success) {
        return $match.Groups[1].Value
    }

    # Older gh versions may not print the run URL. Match the newly-created
    # workflow_dispatch run instead of assuming the newest run is ours.
    for ($attempt = 0; $attempt -lt 12; $attempt++) {
        $runsJson = gh run list --workflow $Workflow --repo $Repo --limit 20 --json databaseId,event,headBranch,createdAt 2>$null
        if ($LASTEXITCODE -eq 0) {
            try {
                $runs = @(ConvertFrom-Json ($runsJson -join "`n"))
                $run = $runs |
                    Where-Object {
                        $_.event -eq "workflow_dispatch" -and
                        $_.headBranch -eq $Ref -and
                        ([datetime]$_.createdAt).ToUniversalTime() -ge $TriggeredAt.ToUniversalTime()
                    } |
                    Sort-Object { [datetime]$_.createdAt } -Descending |
                    Select-Object -First 1
                if ($null -ne $run) {
                    return [string]$run.databaseId
                }
            } catch {
                # The run list can be briefly incomplete while GitHub creates it.
            }
        }
        Start-Sleep -Seconds 5
    }

    throw "Could not determine the triggered run id from: $TriggerOutput"
}

Assert-GhAvailable

Write-Host "Triggering workflow '$Workflow' on $Repo (ref: $Ref, mihomo_version: $MihomoVersion) ..." -ForegroundColor Cyan
$triggeredAt = [datetime]::UtcNow
$output = gh workflow run "$Workflow" --repo $Repo --ref $Ref -f "mihomo_version=$MihomoVersion" 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to trigger the workflow:`n$output"
    exit 1
}

$runId = Get-TriggeredRunId -TriggerOutput ($output -join "`n") -Repo $Repo -Workflow $Workflow -TriggeredAt $triggeredAt -Ref $Ref
$runUrl = "https://github.com/$Repo/actions/runs/$runId"
Write-Host "Run started: $runUrl" -ForegroundColor Green

if ($NoWait) {
    Write-Host "NoWait specified - not watching. Check progress at: $runUrl"
    exit 0
}

Write-Host "Watching run $runId ... (press Ctrl+C to detach)" -ForegroundColor Cyan
gh run watch "$runId" --repo $Repo --interval $Interval --exit-status
$watchExit = $LASTEXITCODE

if (-not $SkipView) {
    Write-Host "`n--- Run summary ---"
    gh run view "$runId" --repo $Repo
}

if ($watchExit -ne 0) {
    Write-Error "Workflow run $runId FAILED (gh exit code: $watchExit). See: $runUrl"
    exit $watchExit
}

Write-Host "Workflow run $runId completed successfully." -ForegroundColor Green
exit 0