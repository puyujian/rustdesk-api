param(
  [Parameter(Mandatory = $false)]
  [string]$BaseUrl = 'http://127.0.0.1:21114',

  [Parameter(Mandatory = $false)]
  [string]$Username,

  [Parameter(Mandatory = $false)]
  [string]$Password,

  [Parameter(Mandatory = $false)]
  [int]$PlanId = 0
)

$ErrorActionPreference = 'Stop'

function Require-Value {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Name,

    [Parameter(Mandatory = $false)]
    [string]$Value
  )

  if ([string]::IsNullOrWhiteSpace($Value)) {
    return Read-Host "请输入 $Name"
  }
  return $Value
}

function Get-ErrMsg {
  param([object]$Resp)

  if ($null -ne $Resp -and $Resp.PSObject -and ($Resp.PSObject.Properties.Name -contains 'message') -and $Resp.message) {
    return [string]$Resp.message
  }
  return 'unknown'
}

$BaseUrl = $BaseUrl.TrimEnd('/')
$Username = Require-Value -Name '用户名' -Value $Username
$Password = Require-Value -Name '密码' -Value $Password

# 1) 登录获取 Bearer token
$loginBody = @{ username = $Username; password = $Password } | ConvertTo-Json
$login = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/login" -ContentType 'application/json' -Body $loginBody
if (-not $login.access_token) {
  throw '登录失败：未返回 access_token'
}
$headers = @{ Authorization = "Bearer $($login.access_token)" }

# 2) 获取可用套餐
$plansResp = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/subscription/plans" -Headers $headers
if ($plansResp.code -ne 0) {
  throw ("获取套餐失败：{0}" -f (Get-ErrMsg $plansResp))
}
$plans = @($plansResp.data)
Write-Output ("可用套餐数量: {0}" -f $plans.Count)
if ($plans.Count -eq 0) {
  throw '无可用套餐：请先在管理端创建并启用订阅套餐'
}

if ($PlanId -le 0) {
  $PlanId = [int]$plans[0].id
}
Write-Output ("使用 plan_id: {0}" -f $PlanId)

# 3) 创建订单，拿到 pay_url / out_trade_no
$orderBody = @{ plan_id = $PlanId } | ConvertTo-Json
$orderResp = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/subscription/orders" -Headers $headers -ContentType 'application/json' -Body $orderBody
if ($orderResp.code -ne 0) {
  throw ("创建订单失败：{0}" -f (Get-ErrMsg $orderResp))
}

$outTradeNo = $orderResp.data.out_trade_no
$payUrl = $orderResp.data.pay_url
if (-not $outTradeNo) {
  throw '创建订单失败：未返回 out_trade_no'
}
if (-not $payUrl) {
  throw '创建订单失败：未返回 pay_url'
}
Write-Output ("out_trade_no: {0}" -f $outTradeNo)

# 仅打印 host/path，避免泄露 sign 等 query 参数
$payUri =
  if ($payUrl.StartsWith('/')) {
    [Uri]::new($BaseUrl + $payUrl)
  } else {
    [Uri]$payUrl
  }
Write-Output ("pay_url host/path: {0}://{1}{2}" -f $payUri.Scheme, $payUri.Host, $payUri.AbsolutePath)

# 4) 如果 pay_url 指向本服务的中转页，则检查是否为 POST 表单并指向 /pay/submit.php
if ($payUri.AbsolutePath -like '*/api/payment/submit') {
  try {
    $resp = Invoke-WebRequest -Method Get -Uri $payUri.AbsoluteUri -UseBasicParsing
  } catch {
    $resp = Invoke-WebRequest -Method Get -Uri $payUri.AbsoluteUri
  }

  $html = $resp.Content

  if ($html -notmatch '(?is)<form[^>]*method\\s*=\\s*\"post\"') {
    throw '支付中转页检查失败：未找到 method=\"post\" 的 form'
  }
  if ($html -notmatch '(?is)pay/submit\\.php') {
    throw '支付中转页检查失败：form action 未指向 /pay/submit.php'
  }

  Write-Output '支付中转页检查通过：已生成 POST 表单并指向 /pay/submit.php'
} else {
  Write-Output 'pay_url 非本服务中转页：如果出现 404，建议升级后端以使用 /api/payment/submit 中转（POST 提交到网关）。'
}

