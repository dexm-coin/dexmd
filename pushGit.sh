sudo rm -rf .dexm*
rm config.json
touch config.json
rm dexmd
git add .
git commit -m "$1"
git push