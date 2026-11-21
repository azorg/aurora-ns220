set +e
BN=/dev/block/platform/mtk-msdc.0/by-name
SD=/run/media/defaultuser/8f89560b-07c5-4287-b5a9-0ac16f7270c1
OUT=$SD/aurora-backup/partitions-all
mkdir -p $OUT
for p in $(ls $BN); do
  sz=$(cat /sys/class/block/$(basename $(readlink -f $BN/$p))/size)
  if [ $sz -lt 20000000 ]; then
    dd if=$BN/$p of=$OUT/$p.img bs=1M 2>/dev/null
  fi
done
echo "== grep for ima cmdline tokens"
grep -a -l 'ima_appraise' $OUT/*.img 2>/dev/null
grep -a -o -E '[ -~]{10,}' $OUT/*.img 2>/dev/null | grep -aE 'ima_appraise|module\.sig_enforce|clearmem' | head -10
du -sh $OUT
