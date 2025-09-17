include $(TOPDIR)/rules.mk

PKG_NAME:=rostovvpn-core
PKG_VERSION:=3.2.0
PKG_RELEASE:=1

PKG_SOURCE:=$(PKG_NAME)-$(PKG_VERSION).tar.gz
PKG_SOURCE_URL:=https://codeload.github.com/Darkmen203/rostovvpn-core/tar.gz/v$(PKG_VERSION)?
PKG_HASH:=ff4c63e9d67a1f2e5d1c24a5c58b1693958bbbd66ad01ef6367697a6c05781b5

PKG_LICENSE:=https://github.com/Darkmen203/RostovVPN/edit/main/LICENSE.md
PKG_LICENSE_FILES:=LICENSE
PKG_MAINTAINER:=RostovVPN <support@rostovvpn.local>

PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1
PKG_BUILD_FLAGS:=no-mips16

GO_PKG:=github.com/Darkmen203/rostovvpn-core
GO_PKG_BUILD_PKG:=$(GO_PKG)/cli

GO_PKG_LDFLAGS_X:=$(GO_PKG)/constant.Version=$(PKG_VERSION)

include $(INCLUDE_DIR)/package.mk
include ../../lang/golang/golang-package.mk

define Package/rostovvpn-core
  TITLE:=RostovVPN universal proxy platform
  SECTION:=net
  CATEGORY:=Network
  URL:=https://rostov.darkmen203.ru/
  DEPENDS:=$(GO_ARCH_DEPENDS) +ca-bundle +kmod-inet-diag +kmod-tun
  USERID:=rostovvpn=61566:rostovvpn=61566
endef

define Package/rostovvpn-core/description
  RostovVPN is a universal proxy platform which supports hysteria, SOCKS, Shadowsocks,
  ShadowTLS, Tor, trojan, VLess, VMess, WireGuard and so on.
endef

GO_PKG_TAGS:=with_gvisor,with_quic,with_wireguard,with_utls,with_clash_api,with_grpc,with_low_memory,with_conntrack

define Package/rostovvpn-core/conffiles
/etc/config/rostovvpn
/etc/rostovvpn/
endef

define Package/rostovvpn-core/install

    $(INSTALL_DIR) $(1)/usr/bin/
    $(INSTALL_BIN) $(GO_PKG_BUILD_BIN_DIR)/cli $(1)/usr/bin/rostovvpn-cli

    $(INSTALL_DIR) $(1)/etc/rostovvpn
    $(INSTALL_DATA) ./files/rostovvpn.json $(1)/etc/rostovvpn/appconfig.json

    $(INSTALL_DIR) $(1)/etc/config/
    $(INSTALL_CONF) ./files/rostovvpn.conf $(1)/etc/config/rostovvpn

    $(INSTALL_DIR) $(1)/etc/init.d/
    $(INSTALL_BIN) ./files/rostovvpn.init $(1)/etc/init.d/rostovvpn

	$(CP) ./files/webui $(1)/usr/bin/
endef

$(eval $(call BuildPackage,rostovvpn-core))

