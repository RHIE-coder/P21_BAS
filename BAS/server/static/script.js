console.log("자바스크립트 불러오기 성공")

const show_json = document.getElementsByClassName("show-json")[0];
const newDIV = document.createElement( 'div' );

fetch('http://localhost:5000/json')
  .then(function(response) {
    return response.json();
  })
  .then(function(data) {
    console.log(data) //{str: "Hello", num: 100}
    
    newDIV.innerHTML = data.str + ", " + data.num;
    newDIV.setAttribute("class","myDiv");
    show_json.appendChild(newDIV);
    
  });