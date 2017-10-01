#!/bin/bash
#
# Description: Expose metrics from apt updates.
#
# Author: Ben Kochie <superq@gmail.com>

echo '# HELP apt_upgrades_pending Apt package pending updates by origin.'
echo '# TYPE apt_upgrades_pending gauge'
/usr/bin/apt-get --just-print full-upgrade \
  | /usr/bin/awk '/^Inst/ {print $5, $6}' \
  | /usr/bin/sort \
  | /usr/bin/uniq -c \
  | awk '{ gsub("\\\\", "\\\\", $2); gsub("\"", "\\\"", $2);
           gsub("\[", "", $3); gsub("\]", "", $3);
           print "apt_upgrades_pending{origin=\"" $2 "\",arch=\"" $3 "\"} " $1 }'

echo '# HELP node_reboot_required Node reboot is required for software updates.'
echo '# TYPE node_reboot_required gauge'
if [[ -f '/run/reboot-required' ]] ; then
  echo 'node_reboot_required 1'
else
  echo 'node_reboot_required 0'
fi
