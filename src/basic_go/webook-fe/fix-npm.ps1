Write-Output "开始修复 npm 环境..."

if (Test-Path "node_modules") {
    Remove-Item -Recurse -Force -ErrorAction Ignore "node_modules"
}
if (Test-Path "package-lock.json") {
    Remove-Item -Force -ErrorAction Ignore "package-lock.json"
}

$NewCache = "C:\Users\$env:USERNAME\npm-cache"
Write-Output "设置 npm 缓存目录到 $NewCache"
npm config set cache $NewCache --global

Write-Output "清理 npm 缓存..."
npm cache clean --force

$CachePath = npm config get cache
Write-Output "当前 npm 缓存目录: $CachePath"

Write-Output "正在重新安装依赖..."
npm install

Write-Output "修复完成！"
