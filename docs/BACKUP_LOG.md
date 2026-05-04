# x-ui 备份记录

## backup-001（第一次备份）

- 备份时间: 2026-04-22 12:31:08 -0400
- 备份编号: `x-ui-source-backup-001`
- 备份文件: `/root/backups/x-ui/x-ui-source-backup-001-20260422-122922.tar.gz`
- 文件大小: `621566341` bytes（约 593M）
- SHA256: `c30b43fe72c3afdfc4e5692d617d68b9b6544fa97bd6d4b53c0594bd162ad8ed`
- 源码目录: `/root/x9526-full`
- 校验文件: `/root/backups/x-ui/x-ui-source-backup-001-20260422-122922.tar.gz.sha256`

## 说明

1. 该条目为 x-ui 首次源码备份记录。  
2. 备份文件已生成，可通过 `sha256sum` 对照校验。  

## backup-002（当前修改版 x-ui 文件第一次本机备份）

- 备份时间: 2026-04-30 12:30:47 -0400
- 备份编号: `x-ui-binary-backup-001`
- 备份文件: `/root/backups/x-ui/x-ui-binary-backup-001-20260430T163025Z`
- 文件大小: `30952224` bytes（约 29.5M）
- SHA256: `42a478c636a83afaa5d5d575b948d6e5e8a49534a6398926ef720c88794f9aea`
- 来源文件: `/root/x-ui-src/x-ui`
- 校验文件: `/root/backups/x-ui/x-ui-binary-backup-001-20260430T163025Z.sha256`

## 补充说明

1. `backup-001` 是更早的源码目录备份记录。  
2. `backup-002` 是当前本机修改完成后的 `x-ui` 可执行文件第一次单文件备份。  

## backup-003（n-ui 第一次完整源码本地备份）

- 备份时间: 2026-04-30 12:41:18 -0400
- 备份编号: `n-ui-source-backup-001`
- 备份文件: `/root/backups/n-ui/n-ui-source-backup-001-20260430T164118Z.tar.gz`
- 文件大小: `81945951` bytes（约 78.1M）
- SHA256: `564a6eebafbbc792ae3492a30442cea5a00e25e7d746b34f01d0aeccdfb5577a`
- 源码目录: `/root/x-ui-src`
- 校验文件: `/root/backups/n-ui/n-ui-source-backup-001-20260430T164118Z.tar.gz.sha256`

## n-ui 说明

1. 该条目按用户当前命名要求，记录为 `n-ui` 第一次完整源码本地备份。  
2. 备份内容来源于当前本机同步完成后的 `/root/x-ui-src`。  

## backup-004（n-ui 第二次完整源码本地备份）

- 备份时间: 2026-04-30 14:30:38 -0400
- 备份编号: `n-ui-source-backup-002`
- 备份文件: `/root/backups/n-ui/n-ui-source-backup-002-20260430T183038Z.tar.gz`
- 文件大小: `81950199` bytes（约 78.2M）
- SHA256: `902fb68dfe8fab22606d530a049e9bc740261e4f412397f0e3164cded6ef0922`
- 源码目录: `/root/x-ui-src`
- 校验文件: `/root/backups/n-ui/n-ui-source-backup-002-20260430T183038Z.tar.gz.sha256`
- 额外副本: `/123/n-ui-source-backup-002-20260430T183038Z.tar.gz`

## 第二次备份说明

1. 该条目记录为 `n-ui` 第二次完整源码本地备份。  
2. 备份内容来源于当前 Trojan 修复并同步完成后的 `/root/x-ui-src`。  
3. 同名备份包已额外复制一份到 `/123/` 目录。  

## backup-005（n-ui 第三次完整源码本地备份，最终版基线）

- 备份时间: 2026-05-01 04:07:13 -0400
- 备份编号: `n-ui-source-backup-003`
- 备份文件: `/root/backups/n-ui/n-ui-source-backup-003-20260501T080713Z.tar.gz`
- 文件大小: `81957918` bytes（约 78.2M）
- SHA256: `57297a34da57e69d562f59e035c13573a2ee4572ea87aa28a14a6fd05aa0adc3`
- 源码目录: `/root/x-ui-src`
- 校验文件: `/root/backups/n-ui/n-ui-source-backup-003-20260501T080713Z.tar.gz.sha256`

## 最终版基线说明

1. 该条目记录为当前阶段清理测试入站后的 `n-ui` 最终版完整源码基线。  
2. 当前常用入站协议已完成源码级兼容性修复与实测。  
3. 后续若继续修改，应以这次备份为新的回滚基线。  

## backup-006（n-ui 第四次完整源码本地备份）

- 备份时间: 2026-05-01 04:17:44 -0400
- 备份编号: `n-ui-source-backup-004`
- 备份文件: `/root/backups/n-ui/n-ui-source-backup-004-20260501T081744Z.tar.gz`
- 文件大小: `81957372` bytes（约 78.2M）
- SHA256: `0e3b7106f580d01826a973b948b8a8ab7a1754dbcafa2f04090a12b63e4e094e`
- 源码目录: `/root/x-ui-src`
- 校验文件: `/root/backups/n-ui/n-ui-source-backup-004-20260501T081744Z.tar.gz.sha256`
- 额外副本: `/123/n-ui-source-backup-004-20260501T081744Z.tar.gz`

## 第四次备份说明

1. 该条目记录为 `n-ui` 第四次完整源码本地备份。  
2. 备份内容来源于隐藏 Trojan 入站选项并同步完成后的 `/root/x-ui-src`。  
3. 同名备份包已额外复制一份到 `/123/` 目录。  

## backup-007（x-ui 第七次完整源码本地备份）

- 备份时间: 2026-05-04 01:03:19 -0400
- 备份编号: `x-ui-source-backup-007`
- 备份文件: `/root/backups/x-ui/x-ui-source-backup-007-20260504T050319Z.tar.gz`
- 文件大小: `125995948` bytes（约 120.2M）
- SHA256: `e505ffab7f0fa9f2c986d0afd44627a9b601d6e288282afc012b4cc57540a0ba`
- 源码目录: `/root/n-ui1`
- 校验文件: `/root/backups/x-ui/x-ui-source-backup-007-20260504T050319Z.tar.gz.sha256`
- 额外副本: `/123/x-ui-source-backup-007-20260504T050319Z.tar.gz`

## 第七次备份说明

1. 该条目记录为当前 `/root/n-ui1` 最终整理版的 `x-ui` 第七次完整源码本地备份。  
2. 备份内容包含完整源码树、`xray-core` 子树、`bin/` 运行依赖、`releases/` 拉取文件包、安装脚本与页面/后端源码改动。  
3. 当前备份对应的访问 IP 最终版、入站端口记录逻辑和本地优先安装/更新逻辑均已纳入。  
4. 同名备份包与 `.sha256` 校验文件已额外复制一份到 `/123/` 目录。  
