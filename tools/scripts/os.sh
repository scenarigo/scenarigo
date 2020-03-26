unameOut="$(uname -s)"
case "${unameOut}" in
    Linux*)     machine=linux;;
    Darwin*)    machine=osx;;
    *)          machine="UNKNOWN:${unameOut}"
esac
echo ${machine}
