# Publishes the dist/ page (index.html + app.js + styles.css) to the gh-pages
# branch and pushes it, so users visiting the URL always see the latest prices.
# Runs right after priceupdate.exe each day (see enable-price-updates.ps1).
# The gh-pages branch is kept as a single-commit snapshot: each run force-replaces
# it, so the repo never accumulates a 10 MB blob per day.
$ErrorActionPreference = 'Stop'
$projectRoot = $PSScriptRoot
$distDir = Join-Path $projectRoot 'dist'
foreach ($f in 'index.html', 'app.js', 'styles.css', 'rows.json') {
    if (!(Test-Path -LiteralPath (Join-Path $distDir $f))) { throw "dist/$f not found; run a price update first." }
}

# Remote is read from the main repo; configure once with:
#   git remote add origin <your-repo-url>
$remote = (git -C $projectRoot remote get-url origin 2>$null)
if (!$remote) { throw "No git remote configured. Run: git remote add origin <repo-url>" }

$publishBranch = 'gh-pages'
# Build a throwaway worktree so the master working tree is never touched.
$work = Join-Path $env:TEMP ('sms-publish-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work | Out-Null
try {
    git -C $work init -q
    git -C $work remote add origin $remote
    git -C $work checkout -q --orphan $publishBranch
    Copy-Item -LiteralPath (Join-Path $distDir 'index.html') -Destination (Join-Path $work 'index.html') -Force
    Copy-Item -LiteralPath (Join-Path $distDir 'app.js') -Destination (Join-Path $work 'app.js') -Force
    Copy-Item -LiteralPath (Join-Path $distDir 'styles.css') -Destination (Join-Path $work 'styles.css') -Force
    Copy-Item -LiteralPath (Join-Path $distDir 'rows.json') -Destination (Join-Path $work 'rows.json') -Force
    # .nojekyll stops GitHub Pages from running Jekyll over the large HTML.
    New-Item -ItemType File -Path (Join-Path $work '.nojekyll') -Force | Out-Null
    git -C $work add index.html app.js styles.css rows.json .nojekyll
    $stamp = (Get-Date).ToUniversalTime().ToString('yyyy-MM-dd HH:mm UTC')
    $identity = git -C $projectRoot config user.name; if (!$identity) { $identity = 'sms-publish' }
    $email = git -C $projectRoot config user.email; if (!$email) { $email = 'sms-publish@localhost' }
    git -C $work -c user.name=$identity -c user.email=$email commit -q -m "publish prices $stamp"
    # Force-push a single snapshot: the remote gh-pages always holds only the latest page.
    git -C $work push -f -q origin "HEAD:$publishBranch"
    Write-Host "Published dist/ page to $publishBranch ($stamp)."
}
finally {
    Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}
