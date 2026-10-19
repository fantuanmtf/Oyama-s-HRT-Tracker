# 完整备份恢复演示（Linux）

这段动画展示的是**离线 Tools** 的恢复流程：网站导出未加密的完整备份 JSON，
`hrt-cold` 在本地用 OpenPGP 公钥加密成 `.hrtb`，再解密回 JSON，最后导入网站。
动画里的数据、指纹和口令都是合成的；它不是真实用户数据，也不是生产环境。

![hrt-cold Linux recovery demo](assets/recovery-demo.gif)

> 如果动画没有加载，可以查看静态图：
> [`assets/recovery-demo-poster.png`](assets/recovery-demo-poster.png)。

## 它做了什么

```sh
# 1. 生成离线密钥（私钥和口令分开保存）
hrt-cold keygen \
  --private-out backup-private.asc \
  --public-out backup-public.asc \
  --passphrase-file passphrase.txt

# 2. 加密网站导出的完整备份
hrt-cold encrypt \
  --recipient backup-public.asc \
  --in website-export.json \
  --out backup.hrtb

# 3. 只看容器元数据，不解密
hrt-cold inspect \
  --in backup.hrtb \
  --recipient backup-public.asc

# 4. 解密，写回网站可导入的 JSON
hrt-cold decrypt \
  --private backup-private.asc \
  --passphrase-file passphrase.txt \
  --in backup.hrtb \
  --out restored.json

# 5. 验证恢复结果与原始导出完全一致
cmp website-export.json restored.json && echo RECOVERY_BYTES_IDENTICAL
```

## 安全边界

- **网站不加密完整备份**；导出的 JSON 是明文健康数据。请只在离线 Tools 中加密后存储或传输。
- `.hrtb` 目前是实验格式，**没有签名、来源认证或防重放**。任何拿到公钥的人都能生成合法密文；
  `inspect` 显示的 recipient key ID 只是 advertised 值，`recipientVerified` 始终是 `false`。
- 请通过独立渠道核对公钥指纹；私钥、口令和备份文件要分开保存。
- 解密失败、错误私钥/口令、篡改、截断、尾部多余 packet/字节都会 fail closed，不写输出文件。
- 详细的 Linux 恢复测试记录见
  [`docs/backup-schema-audit/LINUX_RECOVERY_E2E.md`](backup-schema-audit/LINUX_RECOVERY_E2E.md)；
  本地渗透测试记录见
  [`docs/backup-schema-audit/LOCAL_SECURITY_PENTEST_REPORT.md`](backup-schema-audit/LOCAL_SECURITY_PENTEST_REPORT.md)。

## 这段动画是怎么来的

这是一段用实际命令输出渲染的合成终端动画（synthetic animation），不是屏幕录制；
数据来自 `docs/backup-schema-audit/LINUX_RECOVERY_E2E.md` 中的合成恢复测试。
