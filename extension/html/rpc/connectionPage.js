const {  rostovvpnClient } = require('./client.js');
const rostovvpn = require("./rostovvpn_grpc_web_pb.js");

function openConnectionPage() {
    
        $("#extension-list-container").show();
        $("#extension-page-container").hide();
        $("#connection-page").show();
        connect();
        $("#connect-button").click(async () => {
            const hsetting_request = new rostovvpn.ChangeRostovVPNSettingsRequest();
            hsetting_request.setrostovVPNSettingsJson($("#rostovvpn-settings").val());
            try{
                const hres=await rostovvpnClient.ChangeRostovVPNSettings(hsetting_request, {});
            }catch(err){
                $("#rostovvpn-settings").val("")
                console.log(err)
            }
            
            const parse_request = new rostovvpn.ParseRequest();
            parse_request.setContent($("#config-content").val());
            try{
                const pres=await rostovvpnClient.parse(parse_request, {});
                if (pres.getResponseCode() !== rostovvpn.ResponseCode.OK){
                    alert(pres.getMessage());
                    return
                }
                $("#config-content").val(pres.getContent());
            }catch(err){
                console.log(err)
                alert(JSON.stringify(err))
                                return
            }

            const request = new rostovvpn.StartRequest();
    
            request.setConfigContent($("#config-content").val());
            request.setEnableRawConfig(false);
            try{
                const res=await rostovvpnClient.start(request, {});
                console.log(res.getCoreState(),res.getMessage())
                    handleCoreStatus(res.getCoreState());
            }catch(err){
                console.log(err)
                alert(JSON.stringify(err))
                return
            }

            
        })

        $("#disconnect-button").click(async () => {
            const request = new rostovvpn.Empty();
            try{
                const res=await rostovvpnClient.stop(request, {});
                console.log(res.getCoreState(),res.getMessage())
                handleCoreStatus(res.getCoreState());
            }catch(err){
                console.log(err)
                alert(JSON.stringify(err))
                return
            }
        })
}


function connect(){
    const request = new rostovvpn.Empty();
    const stream = rostovvpnClient.coreInfoListener(request, {});
    stream.on('data', (response) => {
        console.log('Receving ',response);
        handleCoreStatus(response);
    });
    
    stream.on('error', (err) => {
        console.error('Error opening extension page:', err);
        // openExtensionPage(extensionId);
    });
    
    stream.on('end', () => {
        console.log('Stream ended');
        setTimeout(connect, 1000);
        
    });
}


function handleCoreStatus(status){
    if (status == rostovvpn.CoreState.STOPPED){
        $("#connection-before-connect").show();
        $("#connection-connecting").hide();
    }else{
        $("#connection-before-connect").hide();
        $("#connection-connecting").show();
        if (status == rostovvpn.CoreState.STARTING){
            $("#connection-status").text("Starting");
            $("#connection-status").css("color", "yellow");
        }else if (status == rostovvpn.CoreState.STOPPING){
            $("#connection-status").text("Stopping");
            $("#connection-status").css("color", "red");
        }else if (status == rostovvpn.CoreState.STARTED){
            $("#connection-status").text("Connected");
            $("#connection-status").css("color", "green");
        }
    }
}


module.exports = { openConnectionPage };