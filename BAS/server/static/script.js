console.log("자바스크립트 불러오기 성공")


const codeButton = document.getElementsByClassName("code")[0];
const tokenButton = document.getElementsByClassName("token")[0];
const contentTitle = document.getElementsByClassName("content-title")[0];
const contentList = document.getElementsByClassName("content-list")[0];

function loadJSON(location, callback) {   
  var xhr = new XMLHttpRequest();
  xhr.overrideMimeType("application/json");
  xhr.open('GET', location);
  xhr.onreadystatechange = function () {
    if (xhr.readyState == 4 && xhr.status == "200") {
      callback(JSON.parse(xhr.responseText));
    }
  };
  xhr.send(null);  
}

function showJsonDOM(attrList, data){
  for(let i = 0; i < data.length; i++){
    let itemElement = document.createElement("div");
    itemElement.setAttribute("class","content-items");
    for(const attrName of attrList){
      let section = document.createElement("section");
      let div = document.createElement("div");
      div.innerText = attrName;
      let p = document.createElement("p");
      p.innerText = data[i][attrName];
      section.appendChild(div); 
      section.appendChild(p);
      itemElement.appendChild(section);
      contentList.appendChild(itemElement);
    }
  }

}

codeButton.addEventListener('click',function(){
  contentTitle.textContent = "CODE INFORMATION"
  contentList.innerHTML= null
  
  loadJSON('http://localhost:5000/code',function(data){
    let attrList = [
      "InfoType", "ID_code", "DID_RO","DID_client","Scope",
      "Hash_code","Time_issueed","URI_Redirection","Condition","ID_token"];
    
    showJsonDOM(attrList, JSON.parse(data))
  })
})


tokenButton.addEventListener('click',function(){
  contentTitle.textContent = "TOKEN INFORMATION"
  contentList.innerHTML= null
  loadJSON('http://localhost:5000/token',function(data){
    let attrList = [
      "InfoType","ID_token","DID_RO","DID_client","Scope",
      "Hash_code","Time_issueed","Time_expiration","URI_Redirection","Condition"];
    
    showJsonDOM(attrList, JSON.parse(data))
  })
})