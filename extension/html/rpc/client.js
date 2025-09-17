const rostovvpn = require("./rostovvpn_grpc_web_pb.js");
const extension = require("./extension_grpc_web_pb.js");

const grpcServerAddress = '/';
const extensionClient = new extension.ExtensionHostServicePromiseClient(grpcServerAddress, null, null);
const rostovvpnClient = new rostovvpn.CorePromiseClient(grpcServerAddress, null, null);

module.exports = { extensionClient ,rostovvpnClient};