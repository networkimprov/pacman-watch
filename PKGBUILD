# Maintainer: Thomas Dziedzic <gostrc@gmail.com>

pkgname=pacman-watch
pkgver=0.2
pkgrel=1
pkgdesc="pacman watch client"
arch=('armv7h')
url="https://github.com/networkimprov/pacman-watch"
license=('unknown')
backup=('etc/pacman-watch.conf')
source=('pacman-watch-close'
        'pacman-watch-close.service'
        'pacman-watch-open'
        'pacman-watch-open.service'
        'pacman-watch-open.timer'
        'pacman-watch.conf')
md5sums=('5045ec1b7488fc51fcd3b3e76a19da22'
         '50dd1f0b79e959221a63e3f8e210690f'
         '89a7696f35c41ec3f7771ce4c279bcf0'
         'ebb7e4353ea9d8020ab64668047793c5'
         '766ab2cf05523b3825a75bc8de90f683'
         'df8cd1d7393c1f4901a9775bc305fb76')

package() {
  install -d "${pkgdir}/usr/bin"
  install -m 755 pacman-watch-close \
    "${pkgdir}/usr/bin/"
  install -m 755 pacman-watch-open \
    "${pkgdir}/usr/bin/"

  install -d "${pkgdir}/usr/lib/systemd/system"
  install -m 644 pacman-watch-close.service \
    "${pkgdir}/usr/lib/systemd/system/"
  install -m 644 pacman-watch-open.service \
    "${pkgdir}/usr/lib/systemd/system/"
  install -m 644 pacman-watch-open.timer \
    "${pkgdir}/usr/lib/systemd/system/"

  install -d "${pkgdir}/etc"
  install -m 644 pacman-watch.conf \
    "${pkgdir}/etc/"
}
